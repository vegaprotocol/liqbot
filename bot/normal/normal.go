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
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	"github.com/vegaprotocol/api/grpc/clients/go/txn"
	"go.uber.org/zap"
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

	balanceGeneral uint64
	balanceMargin  uint64
	balanceBond    uint64

	walletServer     wallet.WalletHandler
	walletPassphrase string
	walletPubKeyRaw  []byte // "XYZ" ...
	walletPubKeyHex  string // "58595a" ...
	walletToken      string
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func abs(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
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

	b.strategy, err = validateStrategyConfig(config.StrategyDetails)
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

	b.balanceGeneral = 0
	b.balanceMargin = 0

	b.active = true
	b.stopPosMgmt = make(chan bool)
	// b.stopPriceSteer = make(chan bool)

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
	prepared, err := txn.PrepareLiquidityProvision(sub)
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
	prepared, err := txn.PrepareSubmitOrder(sub)
	if err != nil {
		return errors.Wrap(err, "failed to prepare tx")
	}

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_COMMIT)
	if err != nil {
		return errors.Wrap(err, "failed to sign and submit tx")
	}
	return nil
}

func (b *Bot) sendLiquidityProvision(buys, sells []*proto.LiquidityOrder) error {
	// CommitmentAmount is the fractional commitment value * total collateral
	commitment := b.strategy.CommitmentFraction * float64(b.balanceGeneral+b.balanceMargin+b.balanceBond)

	sub := &api.PrepareLiquidityProvisionRequest{
		Submission: &proto.LiquidityProvisionSubmission{
			Fee:              b.config.StrategyDetails.Fee,
			MarketId:         b.market.Id,
			CommitmentAmount: uint64(commitment),
			Buys:             buys,
			Sells:            sells,
		},
	}
	err := b.submitLiquidityProvision(sub)
	if err != nil {
		return errors.Wrap(err, "failed to submit liquidity provision order")
	}
	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       b.balanceGeneral + b.balanceMargin + b.balanceBond,
	}).Debug("Submitted liquidity provision order")
	return nil
}

// getAccountGeneral get this bot's general account balance.
func (b *Bot) getAccountGeneral() error {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		// MarketId: general account is not per market
		PartyId: b.walletPubKeyHex,
		Asset:   b.settlementAsset,
		Type:    proto.AccountType_ACCOUNT_TYPE_GENERAL,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get general account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero general accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many general accounts for party: %d", len(response.Accounts))
	}
	b.balanceGeneral = response.Accounts[0].Balance
	return nil
}

// getAccountMargin get this bot's margin account balance.
func (b *Bot) getAccountMargin() error {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
		Asset:    b.settlementAsset,
		Type:     proto.AccountType_ACCOUNT_TYPE_MARGIN,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get margin account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero margin accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many margin accounts for party: %d", len(response.Accounts))
	}
	b.balanceMargin = response.Accounts[0].Balance
	return nil
}

// getAccountBond get this bot's bond account balance.
func (b *Bot) getAccountBond() error {
	b.balanceBond = 0
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
		Asset:    b.settlementAsset,
		Type:     proto.AccountType_ACCOUNT_TYPE_BOND,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get bond account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero bond accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many bond accounts for party: %d", len(response.Accounts))
	}
	b.balanceBond = response.Accounts[0].Balance
	return nil
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

func (b *Bot) waitForGeneralAccountBalance() {
	sleepTime := b.strategy.PosManagementSleepMilliseconds
	for {
		err := b.getAccountGeneral()
		if err != nil {
			b.log.WithFields(log.Fields{
				"error": err.Error(),
			}).Debug("Failed to get general balance")
		} else {
			if b.balanceGeneral > 0 {
				b.log.WithFields(log.Fields{
					"general": b.balanceGeneral,
				}).Debug("Fetched general balance")
				break
			} else {
				b.log.WithFields(log.Fields{
					"asset": b.settlementAsset,
				}).Warning("Waiting for positive general balance")
			}
		}

		if sleepTime < 9000 {
			sleepTime += 1000
		}
		time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	}
}

func (b *Bot) runPositionManagement() {
	var buyShape, sellShape []*proto.LiquidityOrder
	var marketData *proto.MarketData
	var positions []*proto.Position
	var err error
	var currentPrice uint64
	var openVolume int64
	var previousOpenVolume int64
	var firstTime bool = true

	// We always start off with longening shapes
	buyShape = b.strategy.LongeningShape.Buys
	sellShape = b.strategy.LongeningShape.Sells

	sleepTime := b.strategy.PosManagementSleepMilliseconds
	for {
		select {
		case <-b.stopPosMgmt:
			b.log.Debug("Stopping bot position management")
			b.active = false
			return

		default:
			// At the start of each loop, wait for positive general account balance. This is in case the network has
			// been restarted.
			b.waitForGeneralAccountBalance()

			err = b.getAccountMargin()
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to get margin account balance")
			}

			err = b.getAccountBond()
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to get bond account balance")
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

			if firstTime {
				// Turn the shapes into a set of orders scaled by commitment
				obligation := b.strategy.CommitmentFraction * float64(b.balanceMargin+b.balanceBond+b.balanceGeneral)
				buyOrders := b.CalculateOrderSizes(b.config.MarketID, b.walletPubKeyHex, obligation, buyShape, marketData.MidPrice)
				sellOrders := b.CalculateOrderSizes(b.config.MarketID, b.walletPubKeyHex, obligation, sellShape, marketData.MidPrice)

				buyRisk := float64(0.01)
				sellRisk := float64(0.01)

				buyCost := b.CalculateMarginCost(buyRisk, marketData.MarkPrice, buyOrders)
				sellCost := b.CalculateMarginCost(sellRisk, marketData.MarkPrice, sellOrders)

				shapeMarginCost := max(buyCost, sellCost)

				avail := int64(float64(b.balanceGeneral) * b.strategy.OrdersFraction)
				cost := int64(float64(shapeMarginCost))
				if avail < cost {
					b.log.WithFields(log.Fields{
						"available":       avail,
						"cost":            cost,
						"missing":         avail - cost,
						"missing_percent": (cost - avail) * 100 / avail,
					}).Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")
					err = errors.New("not enough collateral")
				} else {
					// Submit LP order to market.
					err = b.sendLiquidityProvision(buyShape, sellShape)
					if err != nil {
						b.log.Error("Failed to send liquidity provision order", zap.Error(err))
					}
					firstTime = false
				}
			}

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
					buyShape = b.strategy.LongeningShape.Buys
					sellShape = b.strategy.LongeningShape.Sells
				} else {
					shape = "shortening"
					buyShape = b.strategy.ShorteningShape.Buys
					sellShape = b.strategy.ShorteningShape.Sells
				}

				b.log.WithFields(log.Fields{
					"currentPrice":   currentPrice,
					"balanceGeneral": b.balanceGeneral,
					"balanceMargin":  b.balanceMargin,
					"openVolume":     openVolume,
					"shape":          shape,
				}).Debug("Position management info")

				// If we flipped then send the new LP order
				if (openVolume > 0 && previousOpenVolume <= 0) ||
					(openVolume < 0 && previousOpenVolume >= 0) {

					err = b.sendLiquidityProvision(buyShape, sellShape)
					if err != nil {
						b.log.WithFields(log.Fields{
							"error": err.Error(),
						}).Warning("Failed to send liquidity provision")
					}
				}
				previousOpenVolume = openVolume
			}

			if err == nil {
				posMarginCost := calculatePositionMarginCost(openVolume, currentPrice, nil)
				var shouldBuy, shouldSell bool
				if posMarginCost > uint64((1.0-b.strategy.StakeFraction-b.strategy.OrdersFraction)*float64(b.balanceGeneral)) {
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
					request := &api.PrepareSubmitOrderRequest{
						Submission: &proto.OrderSubmission{
							MarketId:    b.market.Id,
							PartyId:     b.walletPubKeyHex,
							Size:        uint64(float64(abs(openVolume)) * b.strategy.PosManagementFraction),
							Side:        proto.Side_SIDE_BUY,
							TimeInForce: proto.Order_TIME_IN_FORCE_IOC,
							Type:        proto.Order_TYPE_MARKET,
							Reference:   "PosManagement",
						},
					}
					err = b.submitOrder(request)
					if err != nil {
						b.log.WithFields(log.Fields{
							"error": err.Error(),
						}).Warning("Failed to submit order")
					}
				} else if shouldSell {
					request := &api.PrepareSubmitOrderRequest{
						Submission: &proto.OrderSubmission{
							MarketId:    b.market.Id,
							PartyId:     b.walletPubKeyHex,
							Size:        uint64(float64(abs(openVolume)) * b.strategy.PosManagementFraction),
							Side:        proto.Side_SIDE_SELL,
							TimeInForce: proto.Order_TIME_IN_FORCE_IOC,
							Type:        proto.Order_TYPE_MARKET,
							Reference:   "PosManagement",
						},
					}
					err = b.submitOrder(request)
					if err != nil {
						b.log.WithFields(log.Fields{
							"error": err.Error(),
						}).Warning("Failed to submit order")
					}
				}
			}

			if err == nil {
				sleepTime = b.strategy.PosManagementSleepMilliseconds
			} else {
				if sleepTime < 29000 {
					sleepTime += 1000
				}
				b.log.WithFields(log.Fields{
					"error":     err.Error(),
					"sleepTime": sleepTime,
				}).Warning("Error during position management")
			}

			err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
			if err != nil {
				b.log.Debug("Stopping bot position management")
				b.active = false
				return
			}
		}
	}
}

// CalculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice)
func (b *Bot) CalculateOrderSizes(marketID, partyID string, obligation float64, liquidityOrders []*proto.LiquidityOrder, midPrice uint64) []*proto.Order {
	orders := make([]*proto.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	var totalProportion uint32
	for _, order := range liquidityOrders {
		totalProportion += order.Proportion
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		prob := 0.10 // Need to make this more accurate later
		fraction := float64(lo.Proportion) / float64(totalProportion)
		scaling := fraction / prob
		size := uint64(math.Ceil(obligation * scaling / float64(midPrice)))

		//		b.log.Debugf("Sizing Info: Size:%d Offset:%d\n", size, lo.Offset)

		peggedOrder := proto.PeggedOrder{
			Reference: lo.Reference,
			Offset:    lo.Offset,
		}

		order := proto.Order{
			MarketId:    marketID,
			PartyId:     partyID,
			Side:        proto.Side_SIDE_BUY,
			Remaining:   size,
			Size:        size,
			TimeInForce: proto.Order_TIME_IN_FORCE_GTC,
			Type:        proto.Order_TYPE_LIMIT,
			PeggedOrder: &peggedOrder,
		}
		orders = append(orders, &order)
	}
	return orders
}

// CalculateMarginCost estimates the margin cost of the set of orders
func (b *Bot) CalculateMarginCost(risk float64, markPrice uint64, orders []*proto.Order) uint64 {
	var totalMargin uint64
	for _, order := range orders {
		if order.Side == proto.Side_SIDE_BUY {
			totalMargin += uint64((risk * float64(markPrice)) + (float64(order.Size) * risk * float64(markPrice)))
		} else {
			totalMargin += uint64(float64(order.Size) * risk * float64(markPrice))
		}
	}
	return totalMargin
}

func (b *Bot) runPriceSteering() {
	var currentPrice, externalPrice uint64
	var err error
	var marketData *proto.MarketData
	var externalPriceResponse ppservice.PriceResponse

	ppcfg := ppconfig.PriceConfig{
		Base:   "BTC",
		Quote:  "USD",
		Wander: true,
	}

	sleepTime := 1000.0 / b.strategy.MarketPriceSteeringRatePerSecond
	for {
		select {
		case <-b.stopPriceSteer:
			b.log.Debug("Stopping bot market price steering")
			b.active = false
			return

		default:
			// At the start of each loop, wait for positive general account balance. This is in case the network has
			// been restarted.
			b.waitForGeneralAccountBalance()

			externalPriceResponse, err = b.pricingEngine.GetPrice(ppcfg)
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
				shouldMove := "no"
				// We only want to steer the price if the external and market price
				// are greater than a certain percentage apart
				currentDiff := math.Abs(float64(currentPrice-externalPrice) / float64(externalPrice))
				if currentDiff > b.strategy.MinPriceSteerFraction {
					var side proto.Side
					if externalPrice > currentPrice {
						side = proto.Side_SIDE_BUY
						shouldMove = "UP"
					} else {
						side = proto.Side_SIDE_SELL
						shouldMove = "DN"
					}
					req := &api.PrepareSubmitOrderRequest{
						Submission: &proto.OrderSubmission{
							Id:          "",
							MarketId:    b.market.Id,
							PartyId:     b.walletPubKeyHex,
							Size:        b.strategy.PriceSteerOrderSize,
							Side:        side,
							TimeInForce: proto.Order_TIME_IN_FORCE_IOC,
							Type:        proto.Order_TYPE_MARKET,
							Reference:   "",
						},
					}
					b.log.WithFields(log.Fields{
						"price": req.Submission.Price,
						"size":  req.Submission.Size,
						"side":  req.Submission.Side,
					}).Debug("Submitting order")
					err = b.submitOrder(req)
				}
				b.log.WithFields(log.Fields{
					"currentPrice":  currentPrice,
					"externalPrice": externalPrice,
					"diff":          int(externalPrice) - int(currentPrice),
					"shouldMove":    shouldMove,
				}).Debug("Steering info")
			}

			if err == nil {
				sleepTime = 1000.0 / b.strategy.MarketPriceSteeringRatePerSecond
			} else {
				if sleepTime < 29000 {
					sleepTime += 1000
				}
				b.log.WithFields(log.Fields{
					"error":     err.Error(),
					"sleepTime": sleepTime,
				}).Warning("Error during price steering")
			}

			err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPriceSteer)
			if err != nil {
				b.log.Debug("Stopping bot market price steering")
				b.active = false
				return
			}
		}
	}
}

func (b *Bot) setupWallet() (err error) {
	//	b.walletPassphrase = "DCBAabcd1357!#&*" + b.config.Name
	b.walletPassphrase = "123"

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
	b.log = b.log.WithFields(log.Fields{"pubkey": b.walletPubKeyHex})
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
