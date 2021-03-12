package normal

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net/url"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/node"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

// Node is a Vega gRPC node
//go:generate go run github.com/golang/mock/mockgen -destination mocks/node_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal Node
type Node interface {
	GetAddress() (url.URL, error)

	// Trading
	SubmitTransaction(req *api.SubmitTransactionRequest) (resp *api.SubmitTransactionResponse, err error)

	// Trading Data
	GetVegaTime() (time.Time, error)
	LiquidityProvisions(req *api.LiquidityProvisionsRequest) (response *api.LiquidityProvisionsResponse, err error)
	MarketByID(req *api.MarketByIDRequest) (response *api.MarketByIDResponse, err error)
	MarketDataByID(req *api.MarketDataByIDRequest) (response *api.MarketDataByIDResponse, err error)
	PartyAccounts(req *api.PartyAccountsRequest) (response *api.PartyAccountsResponse, err error)
	PositionsByParty(req *api.PositionsByPartyRequest) (response *api.PositionsByPartyResponse, err error)
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// Bot represents one Normal liquidity bot.
type Bot struct {
	config          config.BotConfig
	active          bool
	log             *log.Entry
	pricingEngine   PricingEngine
	settlementAsset string
	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	strategy        *Strategy
	market          *proto.Market
	node            Node

	walletServer     wallet.WalletHandler
	walletPassphrase string
	walletPubKeyRaw  []byte // "XYZ" ...
	walletPubKeyHex  string // "58595a" ...
	walletToken      string
}

// New returns a new instance of Bot.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) (b *Bot, err error) {
	b = &Bot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine: pe,
		walletServer:  ws,
	}

	b.strategy, err = readStrategyConfig(config.StrategyDetails)
	if err != nil {
		err = errors.Wrap(err, "failed to read strategy details")
		return
	}
	b.log.WithFields(log.Fields{
		"strategy": b.strategy.String(),
	}).Debug("read strategy config")

	return
}

// Start starts the liquidity bot goroutine(s).
func (b *Bot) Start() error {
	err := b.setupWallet()
	if err != nil {
		return errors.Wrap(err, "failed to start bot")
	}

	b.node, err = node.NewGRPCNode(
		url.URL{Host: b.config.Location},
		time.Duration(b.config.ConnectTimeout)*time.Millisecond,
		time.Duration(b.config.CallTimeout)*time.Millisecond,
	)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Vega gRPC node")
	}
	b.log.WithFields(log.Fields{
		"address": b.config.Location,
	}).Debug("Connected to Vega gRPC node")

	marketResponse, err := b.node.MarketByID(&api.MarketByIDRequest{MarketId: b.config.MarketID})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get market %s", b.config.MarketID))
	}
	b.market = marketResponse.Market
	future := b.market.TradableInstrument.Instrument.GetFuture()
	if future == nil {
		return errors.New("market is not a Futures market")
	}
	b.settlementAsset = future.SettlementAsset
	b.log.WithFields(log.Fields{
		"marketID":        b.config.MarketID,
		"settlementAsset": b.settlementAsset,
	}).Debug("Fetched market info")

	b.active = true
	b.stopPosMgmt = make(chan bool)
	b.stopPriceSteer = make(chan bool)

	go b.runPositionManagement()
	go b.runPriceSteering()

	return nil
}

// Stop stops the liquidity bot goroutine(s).
func (b *Bot) Stop() {
	if !b.active {
		return
	}

	b.stopPosMgmt <- true
	close(b.stopPosMgmt)
	b.stopPriceSteer <- true
	close(b.stopPriceSteer)
}

// ConvertSignedBundle converts from trading-core.wallet.SignedBundle to trading-core.proto.api.SignedBundle
func ConvertSignedBundle(sb *wallet.SignedBundle) *proto.SignedBundle {
	/*
		From: wallet.SignedBundle:

		type SignedBundle struct {
			Tx  []byte
			Sig *wallet.Signature
		}

		To: proto.api.SignedBundle:

		type SignedBundle struct {
			Tx  []byte
			Sig *proto.Signature
		}
	*/
	return &proto.SignedBundle{
		Tx: sb.Tx,
		Sig: &proto.Signature{
			Sig:     sb.Sig.Sig,
			Algo:    sb.Sig.Algo,
			Version: sb.Sig.Version,
		},
	}
}

func (b *Bot) signSubmitTx(blob []byte, typ api.SubmitTransactionRequest_Type) error {
	// Sign, using internal wallet server
	blobBase64 := base64.StdEncoding.EncodeToString(blob)
	signedBundle, err := b.walletServer.SignTx(b.walletToken, blobBase64, b.walletPubKeyHex)
	if err != nil {
		return errors.Wrap(err, "failed to sign tx")
	}

	// Submit TX
	sub := &api.SubmitTransactionRequest{
		Tx:   ConvertSignedBundle(&signedBundle),
		Type: typ,
	}
	submitResponse, err := b.node.SubmitTransaction(sub)
	if err != nil {
		return errors.Wrap(err, "failed to submit signed tx")
	}
	if !submitResponse.Success {
		return errors.New("success=false")
	}
	return nil
}

func (b *Bot) submitLiquidityProvision(sub *api.PrepareLiquidityProvisionRequest) error {
	// Prepare tx, without talking to a Vega node
	prepared, err := node.PrepareLiquidityProvision(sub)
	if err != nil {
		return errors.Wrap(err, "failed to prepare tx")
	}

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_COMMIT)
	if err != nil {
		return errors.Wrap(err, "failed to sign and submit tx")
	}
	return nil
}

func (b *Bot) submitOrder(sub *api.PrepareSubmitOrderRequest) error {
	// Prepare tx, without talking to a Vega node
	prepared, err := node.PrepareSubmitOrder(sub)
	if err != nil {
		return errors.Wrap(err, "failed to prepare tx")
	}

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_COMMIT)
	if err != nil {
		return errors.Wrap(err, "failed to sign and submit tx")
	}
	return nil
}

func (b *Bot) manageLiquidityProvision(buys, sells []*proto.LiquidityOrder) error {
	// lpReq := &api.LiquidityProvisionsRequest{
	// 	Market: b.market.Id,
	// 	Party:  b.walletPubKeyHex,
	// }
	// lpResponse, err := b.node.LiquidityProvisions(lpReq)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to get liquidity provisions")
	// }
	// if len(lpResponse.LiquidityProvisions) > 0 {
	// 	b.log.WithFields(log.Fields{
	// 		"count": len(lpResponse.LiquidityProvisions),
	// 	}).Debug("Liquidity provision already exists")
	// 	for i, lp := range lpResponse.LiquidityProvisions {
	// 		b.log.WithFields(log.Fields{
	// 			"i":  i,
	// 			"lp": lp,
	// 		}).Debug("Liquidity provision detail")
	// 	}
	// 	return nil
	// }

	sub := &api.PrepareLiquidityProvisionRequest{
		Submission: &proto.LiquidityProvisionSubmission{
			Fee:              "0.01",
			MarketId:         b.market.Id,
			CommitmentAmount: 1000,
			Buys:             buys,
			Sells:            sells,
		},
	}
	err := b.submitLiquidityProvision(sub)
	if err != nil {
		return errors.Wrap(err, "failed to submit liquidity provision order")
	}
	b.log.Debug("Submitted liquidity provision order")
	return nil
}

// getAccountGeneral get this bot's general account balance.
func (b *Bot) getAccountGeneral() (uint64, error) {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		// MarketId: general account is not per market
		PartyId: b.walletPubKeyHex,
		Asset:   b.settlementAsset,
		Type:    proto.AccountType_ACCOUNT_TYPE_GENERAL,
	})
	if err != nil {
		return 0, errors.Wrap(err, "failed to get general account")
	}
	if len(response.Accounts) == 0 {
		return 0, errors.Wrap(err, "found zero general accounts for party")
	}

	return response.Accounts[0].Balance, nil
}

// getAccountMargin get this bot's margin account balance.
func (b *Bot) getAccountMargin() (uint64, error) {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
		Asset:    b.settlementAsset,
		Type:     proto.AccountType_ACCOUNT_TYPE_MARGIN,
	})
	if err != nil {
		return 0, errors.Wrap(err, "failed to get margin account")
	}
	if len(response.Accounts) == 0 {
		return 0, errors.Wrap(err, "found zero margin accounts for party")
	}

	return response.Accounts[0].Balance, nil
}

// getMarketData get the market data for the market that this bot trades on.
func (b *Bot) getMarketData() (*proto.MarketData, error) {
	response, err := b.node.MarketDataByID(&api.MarketDataByIDRequest{
		MarketId: b.market.Id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get market data")
	}
	return response.MarketData, nil
}

// getPositions get this bot's positions.
func (b *Bot) getPositions() ([]*proto.Position, error) {
	response, err := b.node.PositionsByParty(&api.PositionsByPartyRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get positions by party")
	}
	return response.Positions, nil
}

func calculatePositionMarginCost(openVolume int64, currentPrice uint64, riskParameters *struct{}) uint64 {
	return 1
}

func (b *Bot) runPositionManagement() {
	var generalBalance, marginBalance uint64
	var buys, sells []*proto.LiquidityOrder
	var marketData *proto.MarketData
	var positions []*proto.Position
	var err error
	var currentPrice uint64
	var openVolume int64

	// TODO: extract into config file
	longeningBuys := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -8, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -4, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -2, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -1, Proportion: 25},
	}
	longeningSells := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 2, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 4, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 8, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 16, Proportion: 25},
	}
	shorteningBuys := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -2, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -4, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -8, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -16, Proportion: 25},
	}
	shorteningSells := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 1, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 2, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 4, Proportion: 25},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 8, Proportion: 25},
	}

	for {
		select {
		case <-b.stopPosMgmt:
			b.log.Debug("Stopping bot position management")
			b.active = false
			return

		default:
			generalBalance, err = b.getAccountGeneral()
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to get general account balance")
			}

			if err == nil {
				marginBalance, err = b.getAccountMargin()
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get margin account balance")
				}
			}

			if err == nil {
				marketData, err = b.getMarketData()
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get market data")
				} else {
					currentPrice = marketData.MarkPrice
				}
			}

			// defBuyingShapeMarginCost = CalculateMarginCost(risk model params, currentPrice, defaultBuyingShape)

			// defSellingShapeMarginCost = CalculateMarginCost(risk model params, currentPrice, defaultSellingShape)

			// shapeMarginCost = max(defBuyingShapeMarginCost,defSellingShapeMarginCost)

			// if assetBalance * ordersFraction < shapeMarginCost
			//     throw Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")
			// else
			//     proceed by submitting the LP order with the defaultBuyingShape to the market.

			if err == nil {
				positions, err = b.getPositions()
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get positions")
				} else {
					if len(positions) == 0 {
						openVolume = 0
					} else {
						openVolume = positions[0].OpenVolume
					}
				}
			}

			if err == nil {
				var shape string
				if openVolume <= 0 {
					shape = "longening"
					buys = longeningBuys
					sells = longeningSells
				} else {
					shape = "shortening"
					buys = shorteningBuys
					sells = shorteningSells
				}

				b.log.WithFields(log.Fields{
					"currentPrice":   currentPrice,
					"generalBalance": generalBalance,
					"marginBalance":  marginBalance,
					"openVolume":     openVolume,
					"shape":          shape,
				}).Debug("Position management info")

				err = b.manageLiquidityProvision(buys, sells)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to manage liquidity provision")
				}

			}

			if err == nil {
				posMarginCost := calculatePositionMarginCost(openVolume, currentPrice, nil)
				var shouldBuy, shouldSell bool
				if posMarginCost > uint64((1.0-b.strategy.StakeFraction-b.strategy.OrdersFraction)*float64(generalBalance)) {
					if openVolume > 0 {
						shouldSell = true
					} else if openVolume < 0 {
						shouldBuy = true
					}
				} else if openVolume >= 0 && uint64(openVolume) > b.strategy.MaxLong {
					shouldSell = true
				} else if openVolume < 0 && uint64(-openVolume) > b.strategy.MaxShort {
					shouldBuy = true
				}

				if shouldBuy {
					b.log.Debug("TODO: place a market buy order for posManagementFraction x position volume")
				} else if shouldSell {
					b.log.Debug("TODO: place a market sell order for posManagementFraction x (-position) volume")
				}
			}

			err = doze(time.Duration(b.strategy.PosManagementSleepMilliseconds)*time.Millisecond, b.stopPosMgmt)
			if err != nil {
				b.log.Debug("Stopping bot position management")
				b.active = false
				return
			}
		}
	}
}

func (b *Bot) runPriceSteering() {
	var currentPrice, externalPrice uint64
	var marketData *proto.MarketData

	ppcfg := ppconfig.PriceConfig{
		Base:   "BTC",
		Quote:  "USD",
		Wander: true,
	}

	for {
		select {
		case <-b.stopPriceSteer:
			b.log.Debug("Stopping bot market price steering")
			b.active = false
			return

		default:
			externalPriceResponse, err := b.pricingEngine.GetPrice(ppcfg)
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to get external price")
			} else {
				externalPrice = uint64(externalPriceResponse.Price * math.Pow10(int(b.market.DecimalPlaces)))
			}

			if err == nil {
				marketData, err = b.getMarketData()
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get market data")
				} else {
					currentPrice = marketData.MarkPrice
				}
			}

			if err == nil {
				b.log.WithFields(log.Fields{
					"currentPrice":  currentPrice,
					"externalPrice": externalPrice,
				}).Debug("Steering info")

				expiration := time.Now().Add(time.Duration(1) * time.Minute)

				var side proto.Side
				if externalPrice > currentPrice {
					side = proto.Side_SIDE_BUY
				} else {
					side = proto.Side_SIDE_SELL
				}
				for _, inc := range []uint64{0, 1, 2, 4, 8, 16, 32, 10000} {
					req := &api.PrepareSubmitOrderRequest{
						Submission: &proto.OrderSubmission{
							Id:          "",
							MarketId:    b.market.Id,
							PartyId:     b.walletPubKeyHex,
							Price:       externalPrice + inc,
							Size:        1,
							Side:        side,
							TimeInForce: proto.Order_TIME_IN_FORCE_GTT,
							ExpiresAt:   expiration.UnixNano(),
							Type:        proto.Order_TYPE_LIMIT,
							Reference:   "",
						},
					}
					b.log.WithFields(log.Fields{
						"price":     req.Submission.Price,
						"size":      req.Submission.Size,
						"side":      req.Submission.Side,
						"increment": inc,
					}).Debug("Submitting order")
					err = b.submitOrder(req)
				}
				if externalPrice > currentPrice {
					side = proto.Side_SIDE_SELL
				} else {
					side = proto.Side_SIDE_BUY
				}
				for _, inc := range []uint64{0, 1, 2, 4, 8, 16, 32, 10000} {
					req := &api.PrepareSubmitOrderRequest{
						Submission: &proto.OrderSubmission{
							Id:          "",
							MarketId:    b.market.Id,
							PartyId:     b.walletPubKeyHex,
							Price:       externalPrice - inc,
							Size:        1,
							Side:        side,
							TimeInForce: proto.Order_TIME_IN_FORCE_GTT,
							ExpiresAt:   expiration.UnixNano(),
							Type:        proto.Order_TYPE_LIMIT,
							Reference:   "",
						},
					}
					b.log.WithFields(log.Fields{
						"price":     req.Submission.Price,
						"size":      req.Submission.Size,
						"side":      req.Submission.Side,
						"increment": inc,
					}).Debug("Submitting order")
					err = b.submitOrder(req)
				}
			}

			err = doze(time.Duration(1.0/b.strategy.MarketPriceSteeringRatePerSecond)*time.Second, b.stopPriceSteer)
			if err != nil {
				b.log.Debug("Stopping bot market price steering")
				b.active = false
				return
			}
		}
	}
}

func (b *Bot) setupWallet() (err error) {
	b.walletPassphrase = "DCBAabcd1357!#&*" + b.config.Name

	if b.walletToken == "" {
		b.walletToken, err = b.walletServer.LoginWallet(b.config.Name, b.walletPassphrase)
		if err != nil {
			if err == wallet.ErrWalletDoesNotExists {
				b.walletToken, err = b.walletServer.CreateWallet(b.config.Name, b.walletPassphrase)
				if err != nil {
					return errors.Wrap(err, "failed to create wallet")
				}
				b.log.Debug("Created and logged into wallet")
			} else {
				return errors.Wrap(err, "failed to log in to wallet")
			}
		} else {
			b.log.Debug("Logged into wallet")
		}
	}

	if b.walletPubKeyHex == "" || b.walletPubKeyRaw == nil {
		var keys []wallet.Keypair
		keys, err = b.walletServer.ListPublicKeys(b.walletToken)
		if err != nil {
			return errors.Wrap(err, "failed to list public keys")
		}
		if len(keys) == 0 {
			b.walletPubKeyHex, err = b.walletServer.GenerateKeypair(b.walletToken, b.walletPassphrase)
			if err != nil {
				return errors.Wrap(err, "failed to generate keypair")
			}
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKeyHex}).Debug("Created keypair")
		} else {
			b.walletPubKeyHex = keys[0].Pub
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKeyHex}).Debug("Using existing keypair")
		}

		b.walletPubKeyRaw, err = hexToRaw([]byte(b.walletPubKeyHex))
		if err != nil {
			b.walletPubKeyHex = ""
			b.walletPubKeyRaw = nil
			return errors.Wrap(err, "failed to decode hex pubkey")
		}
	}
	return
}

func doze(d time.Duration, stop chan bool) error {
	interval := 100 * time.Millisecond
	for d > interval {
		select {
		case <-stop:
			return e.ErrInterrupted

		default:
			time.Sleep(interval)
			d -= interval
		}
	}
	time.Sleep(d)
	return nil
}

func hexToRaw(hexBytes []byte) ([]byte, error) {
	raw := make([]byte, hex.DecodedLen(len(hexBytes)))
	n, err := hex.Decode(raw, hexBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hex")
	}
	if n != len(raw) {
		return nil, fmt.Errorf("failed to decode hex: decoded %d bytes, expected to decode %d bytes", n, len(raw))
	}
	return raw, nil
}
