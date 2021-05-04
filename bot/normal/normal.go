package normal

import (
	"encoding/base64"
	"fmt"
	"math"
	"net/url"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/node"
	"go.uber.org/zap"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	"github.com/vegaprotocol/api/grpc/clients/go/txn"
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

	// Events
	ObserveEventBus() (stream api.TradingDataService_ObserveEventBusClient, err error)
	PositionsSubscribe(req *api.PositionsSubscribeRequest) (stream api.TradingDataService_PositionsSubscribeClient, err error)
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

	buyShape   []*proto.LiquidityOrder
	sellShape  []*proto.LiquidityOrder
	marketData *proto.MarketData
	positions  []*proto.Position

	currentPrice       uint64
	openVolume         int64
	previousOpenVolume int64

	// These flags are used for the streaming systems to let
	// the app know if they are up and working
	eventStreamLive    bool
	positionStreamLive bool
}

// New returns a new instance of Bot.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) (b *Bot, err error) {
	b = &Bot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine:      pe,
		walletServer:       ws,
		eventStreamLive:    false,
		positionStreamLive: false,
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
		return errors.Wrap(err, "failed to setup wallet")
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
	if marketResponse.Market == nil {
		return fmt.Errorf("No market that matchs our ID: %s", b.config.MarketID)
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

func calculatePositionMarginCost(openVolume int64, currentPrice uint64, riskParameters *struct{}) uint64 {
	return 1
}

func (b *Bot) checkForShapeChange() {
	var shape string
	if b.openVolume <= 0 {
		shape = "longening"
		b.buyShape = b.strategy.LongeningShape.Buys
		b.sellShape = b.strategy.LongeningShape.Sells
	} else {
		shape = "shortening"
		b.buyShape = b.strategy.ShorteningShape.Buys
		b.sellShape = b.strategy.ShorteningShape.Sells
	}

	b.log.WithFields(log.Fields{
		"currentPrice":   b.currentPrice,
		"balanceGeneral": b.balanceGeneral,
		"balanceMargin":  b.balanceMargin,
		"openVolume":     b.openVolume,
		"shape":          shape,
	}).Debug("Position management info")

	// If we flipped then send the new LP order
	if (b.openVolume > 0 && b.previousOpenVolume <= 0) ||
		(b.openVolume < 0 && b.previousOpenVolume >= 0) {

		err := b.sendLiquidityProvision(b.buyShape, b.sellShape)
		if err != nil {
			b.log.WithFields(log.Fields{
				"error": err.Error(),
			}).Warning("Failed to send liquidity provision")
		}
	}
	b.previousOpenVolume = b.openVolume
}

func (b *Bot) checkPositionManagement() {
	posMarginCost := calculatePositionMarginCost(b.openVolume, b.currentPrice, nil)
	var shouldBuy, shouldSell bool
	if posMarginCost > uint64((1.0-b.strategy.StakeFraction-b.strategy.OrdersFraction)*float64(b.balanceGeneral)) {
		if b.openVolume > 0 {
			shouldSell = true
		} else if b.openVolume < 0 {
			shouldBuy = true
		}
	} else if b.openVolume >= 0 && uint64(b.openVolume) > b.strategy.MaxLong {
		shouldSell = true
	} else if b.openVolume < 0 && uint64(-b.openVolume) > b.strategy.MaxShort {
		shouldBuy = true
	}

	if shouldBuy {
		request := &api.PrepareSubmitOrderRequest{
			Submission: &proto.OrderSubmission{
				MarketId:    b.market.Id,
				PartyId:     b.walletPubKeyHex,
				Size:        uint64(float64(abs(b.openVolume)) * b.strategy.PosManagementFraction),
				Side:        proto.Side_SIDE_BUY,
				TimeInForce: proto.Order_TIME_IN_FORCE_IOC,
				Type:        proto.Order_TYPE_MARKET,
				Reference:   "PosManagement",
			},
		}
		err := b.submitOrder(request)
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
				Size:        uint64(float64(abs(b.openVolume)) * b.strategy.PosManagementFraction),
				Side:        proto.Side_SIDE_SELL,
				TimeInForce: proto.Order_TIME_IN_FORCE_IOC,
				Type:        proto.Order_TYPE_MARKET,
				Reference:   "PosManagement",
			},
		}
		err := b.submitOrder(request)
		if err != nil {
			b.log.WithFields(log.Fields{
				"error": err.Error(),
			}).Warning("Failed to submit order")
		}
	}
}

func (b *Bot) checkInitialMargin() error {
	// Turn the shapes into a set of orders scaled by commitment
	obligation := b.strategy.CommitmentFraction * float64(b.balanceMargin+b.balanceBond+b.balanceGeneral)
	buyOrders := b.calculateOrderSizes(b.config.MarketID, b.walletPubKeyHex, obligation, b.buyShape, b.marketData.MidPrice)
	sellOrders := b.calculateOrderSizes(b.config.MarketID, b.walletPubKeyHex, obligation, b.sellShape, b.marketData.MidPrice)

	buyRisk := float64(0.01)
	sellRisk := float64(0.01)

	buyCost := b.calculateMarginCost(buyRisk, b.marketData.MarkPrice, buyOrders)
	sellCost := b.calculateMarginCost(sellRisk, b.marketData.MarkPrice, sellOrders)

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
		return errors.New("not enough collateral")
	}
	return nil
}

func (b *Bot) initialiseData() error {
	var err error

	err = b.lookupInitialValues()
	if err != nil {
		b.log.Debug("Stopping position management as we could not get initial values")
		return err
	}

	if !b.eventStreamLive {
		err = b.subscribeToEvents()
		if err != nil {
			b.log.Debug("Unable to subscribe to event bus feeds")
			return err
		}
	}

	if !b.positionStreamLive {
		err = b.subscribePositions()
		if err != nil {
			b.log.Debug("Unable to subscribe to event bus feeds")
			return err
		}
	}
	return nil
}

func (b *Bot) runPositionManagement() {
	var err error
	var firstTime bool = true

	err = b.initialiseData()
	if err != nil {
		b.log.Debug("Stopping position management as we could not get initial the data system")
		b.active = false
		return
	}

	// We always start off with longening shapes
	b.buyShape = b.strategy.LongeningShape.Buys
	b.sellShape = b.strategy.LongeningShape.Sells

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
			if firstTime {
				err = b.checkInitialMargin()
				if err != nil {
					b.active = false
					b.log.Error("Failed initial margin check", zap.Error(err))
					return
				}
				// Submit LP order to market.
				err = b.sendLiquidityProvision(b.buyShape, b.sellShape)
				if err != nil {
					b.log.Error("Failed to send liquidity provision order", zap.Error(err))
					return
				}
				firstTime = false
			}

			b.checkForShapeChange()
			b.checkPositionManagement()

			// If we have lost the incoming streams we should attempt to reconnect
			for !b.positionStreamLive || !b.eventStreamLive {
				err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
				if err != nil {
					b.log.Debug("Stopping bot position management")
					b.active = false
					return
				}

				err = b.initialiseData()
				if err != nil {
					continue
				}
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

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice)
func (b *Bot) calculateOrderSizes(marketID, partyID string, obligation float64, liquidityOrders []*proto.LiquidityOrder, midPrice uint64) []*proto.Order {
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

// calculateMarginCost estimates the margin cost of the set of orders
func (b *Bot) calculateMarginCost(risk float64, markPrice uint64, orders []*proto.Order) uint64 {
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
			externalPriceResponse, err = b.pricingEngine.GetPrice(ppcfg)
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to get external price")
			} else {
				externalPrice = uint64(externalPriceResponse.Price * math.Pow10(int(b.market.DecimalPlaces)))
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
