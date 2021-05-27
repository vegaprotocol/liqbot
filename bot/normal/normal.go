package normal

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/node"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	commandspb "github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/commands/v1"
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
	AssetByID(assetID string) (response *api.AssetByIDResponse, err error)

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
	config                 config.BotConfig
	active                 bool
	log                    *log.Entry
	pricingEngine          PricingEngine
	settlementAssetID      string
	settlementAssetAddress string
	stopPosMgmt            chan bool
	stopPriceSteer         chan bool
	strategy               *Strategy
	market                 *proto.Market
	node                   Node

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

	// Flag to indicate if we have already placed auction orders
	auctionOrdersPlaced bool
}

// New returns a new instance of Bot.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) (b *Bot, err error) {
	b = &Bot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine:       pe,
		walletServer:        ws,
		eventStreamLive:     false,
		positionStreamLive:  false,
		auctionOrdersPlaced: false,
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
	b.settlementAssetID = future.SettlementAsset
	b.log.WithFields(log.Fields{
		"marketID":          b.config.MarketID,
		"settlementAssetID": b.settlementAssetID,
	}).Debug("Fetched market info")

	// Use the settlementAssetID to lookup the settlement ethereum address
	assetResponse, err := b.node.AssetByID(b.settlementAssetID)
	if err != nil {
		return fmt.Errorf("unable to look up asset details for %s", b.settlementAssetID)
	}
	erc20 := assetResponse.Asset.Source.GetErc20()
	if erc20 != nil {
		b.settlementAssetAddress = erc20.ContractAddress
	} else {
		b.settlementAssetAddress = ""
	}

	b.balanceGeneral = 0
	b.balanceMargin = 0

	b.active = true
	b.stopPosMgmt = make(chan bool)
	b.stopPriceSteer = make(chan bool)

	err = b.initialiseData()
	if err != nil {
		return fmt.Errorf("failed to initialise data: %w", err)
	}
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

// GetTraderDetails returns information relating to the trader
func (b *Bot) GetTraderDetails() string {
	name := b.config.Name
	pubKey := b.walletPubKeyHex
	settlementVegaAssetID := b.settlementAssetID
	settlementEthereumContractAddress := b.settlementAssetAddress

	return "{\"name\":\"" + name + "\",\"pubKey\":\"" + pubKey + "\",\"settlementVegaAssetID\":\"" +
		settlementVegaAssetID + "\",\"settlementEthereumContractAddress\":\"" +
		settlementEthereumContractAddress + "\"}"
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
		Tx: ConvertSignedBundle(&signedBundle),
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

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_ASYNC)
	if err != nil {
		return errors.Wrap(err, "failed to sign and submit tx")
	}
	return nil
}

func (b *Bot) canPlaceTrades() bool {
	return b.marketData.MarketTradingMode == proto.Market_TRADING_MODE_CONTINUOUS
}

func (b *Bot) submitOrder(sub *api.PrepareSubmitOrderRequest) error {
	// Prepare tx, without talking to a Vega node
	prepared, err := txn.PrepareSubmitOrder(sub)
	if err != nil {
		return errors.Wrap(err, "failed to prepare tx")
	}

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_ASYNC)
	if err != nil {
		return errors.Wrap(err, "failed to sign and submit tx")
	}
	return nil
}

func (b *Bot) sendLiquidityProvision(buys, sells []*proto.LiquidityOrder) error {
	// CommitmentAmount is the fractional commitment value * total collateral
	commitment := b.strategy.CommitmentFraction * float64(b.balanceGeneral+b.balanceMargin+b.balanceBond)

	sub := &api.PrepareLiquidityProvisionRequest{
		Submission: &commandspb.LiquidityProvisionSubmission{
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

		b.log.WithFields(log.Fields{"shape": shape}).Debug("Flipping LP direction")
		err := b.sendLiquidityProvision(b.buyShape, b.sellShape)
		if err != nil {
			b.log.WithFields(log.Fields{
				"error": err.Error(),
			}).Warning("Failed to send liquidity provision")
		} else {
			b.previousOpenVolume = b.openVolume
		}
	}
}

func (b *Bot) checkPositionManagement() {
	if !b.canPlaceTrades() {
		// Only allow position management during continuous trading
		return
	}
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
		size := uint64(float64(abs(b.openVolume)) * b.strategy.PosManagementFraction)
		err := b.sendOrder(size, 0, proto.Side_SIDE_BUY, proto.Order_TIME_IN_FORCE_IOC, proto.Order_TYPE_MARKET, "PosManagement", 0)
		if err != nil {
			log.Warningln("Failed to place a position management buy")
		}
	} else if shouldSell {
		size := uint64(float64(abs(b.openVolume)) * b.strategy.PosManagementFraction)
		err := b.sendOrder(size, 0, proto.Side_SIDE_SELL, proto.Order_TIME_IN_FORCE_IOC, proto.Order_TYPE_MARKET, "PosManagement", 0)
		if err != nil {
			log.Warningln("Failed to place a position management sell")
		}
	}
}

func (b *Bot) sendOrder(
	size, price uint64,
	side proto.Side,
	tif proto.Order_TimeInForce,
	orderType proto.Order_Type,
	reference string,
	secondsFromNow int64,
) error {
	request := &api.PrepareSubmitOrderRequest{
		Submission: &commandspb.OrderSubmission{
			MarketId:    b.market.Id,
			Size:        size,
			Side:        side,
			TimeInForce: tif,
			Type:        orderType,
			Reference:   reference,
		},
	}
	if tif == proto.Order_TIME_IN_FORCE_GTT {
		request.Submission.ExpiresAt = time.Now().UnixNano() + (secondsFromNow * 1000000000)
	}

	if orderType != proto.Order_TYPE_MARKET {
		request.Submission.Price = price
	}

	err := b.submitOrder(request)
	if err != nil {
		b.log.WithFields(log.Fields{
			"error": err.Error(),
		}).Warning("Failed to submit order")
	}
	return err
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
		var missingPercent string
		if avail == 0 {
			missingPercent = "Inf"
		} else {
			missingPercent = fmt.Sprintf("%.2f%%", float32((cost-avail)*100)/float32(avail))
		}
		b.log.WithFields(log.Fields{
			"available":      avail,
			"cost":           cost,
			"missing":        avail - cost,
			"missingPercent": missingPercent,
		}).Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")
		return errors.New("not enough collateral")
	}
	return nil
}

func (b *Bot) initialiseData() error {
	var err error

	err = b.lookupInitialValues()
	if err != nil {
		b.log.Debugf("Stopping position management as we could not get initial values: %v", err)
		return err
	}

	if !b.eventStreamLive {
		err = b.subscribeToEvents()
		if err != nil {
			b.log.Debugf("Unable to subscribe to event bus feeds: %v", err)
			return err
		}
	}

	if !b.positionStreamLive {
		err = b.subscribePositions()
		if err != nil {
			b.log.Debugf("Unable to subscribe to event bus feeds: %v", err)
			return err
		}
	}
	return nil
}

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *Bot) placeAuctionOrders() {
	// Check we have not placed them already
	if b.auctionOrdersPlaced == true {
		return
	}
	// Check we have a currentPrice we can use
	if b.currentPrice == 0 {
		return
	}

	// Place the random orders split into
	var totalVolume uint64
	rand.Seed(time.Now().UnixNano())
	for totalVolume < b.config.StrategyDetails.AuctionVolume {
		remaining := b.config.StrategyDetails.AuctionVolume - totalVolume
		size := min(1+(b.config.StrategyDetails.AuctionVolume/10), remaining)
		price := b.currentPrice + (uint64(rand.Int63n(100) - 50)) // #nosec G404 This suboptimal rand generator is fine for now
		side := proto.Side_SIDE_BUY
		if rand.Intn(2) == 0 { // #nosec G404 This suboptimal rand generator is fine for now
			side = proto.Side_SIDE_SELL
		}
		err := b.sendOrder(size, price, side, proto.Order_TIME_IN_FORCE_GTT, proto.Order_TYPE_LIMIT, "AuctionOrder", 330)
		if err == nil {
			totalVolume += size
		} else {
			// We failed to send an order so stop trying to send anymore
			break
		}
	}
	b.auctionOrdersPlaced = true
}

func (b *Bot) runPositionManagement() {
	var err error
	var firstTime bool = true

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
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Error("Failed initial margin check")
					return
				}
				// Submit LP order to market.
				err = b.sendLiquidityProvision(b.buyShape, b.sellShape)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Error("Failed to send liquidity provision order")
					return
				}
				firstTime = false
			}

			// Only update liquidity and position if we are not in auction
			if b.canPlaceTrades() {
				b.auctionOrdersPlaced = false
				b.checkForShapeChange()
				b.checkPositionManagement()
			} else {
				b.placeAuctionOrders()
			}

			// If we have lost the incoming streams we should attempt to reconnect
			for !b.positionStreamLive || !b.eventStreamLive {
				err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
				if err != nil {
					b.log.Debugf("Stopping bot position management: %v", err)
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
				b.log.Debugf("Stopping bot position management: %v", err)
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

func (b *Bot) getPriceParts() (base, quote string, err error) {
	// If we have been passed in the values, use those
	if len(b.config.InstrumentBase) > 0 &&
		len(b.config.InstrumentQuote) > 0 {
		return b.config.InstrumentBase, b.config.InstrumentQuote, nil
	}

	// Find out the underlying assets for this market
	instrument := b.market.TradableInstrument.GetInstrument()
	if instrument != nil {
		md := instrument.Metadata
		for _, tag := range md.Tags {
			parts := strings.Split(tag, ":")
			if len(parts) == 2 {
				if parts[0] == "quote" {
					quote = parts[1]
				}
				if parts[0] == "base" {
					base = parts[1]
				}
			}
		}
	}
	if len(quote) == 0 || len(base) == 0 {
		return "", "", fmt.Errorf("Unable to work out price assets from market metadata")
	}
	return
}

func (b *Bot) runPriceSteering() {
	var externalPrice, currentPrice uint64
	var err error
	var externalPriceResponse ppservice.PriceResponse

	base, quote, err := b.getPriceParts()
	if err != nil {
		b.log.Fatalf("Unable to build instrument for external price feed: %v", err)
	}

	ppcfg := ppconfig.PriceConfig{
		Base:   base,
		Quote:  quote,
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
			if b.strategy.PriceSteerOrderScale > 0 && b.canPlaceTrades() {
				externalPriceResponse, err = b.pricingEngine.GetPrice(ppcfg)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get external price")
					externalPrice = 0
					currentPrice = 0
				} else {
					externalPrice = uint64(externalPriceResponse.Price * math.Pow10(int(b.market.DecimalPlaces)))
					currentPrice = b.marketData.StaticMidPrice
				}

				if err == nil && externalPrice != 0 {
					shouldMove := "no"
					// We only want to steer the price if the external and market price
					// are greater than a certain percentage apart
					currentDiff := math.Abs((float64(currentPrice) - float64(externalPrice)) / float64(externalPrice))
					if currentDiff > b.strategy.MinPriceSteerFraction {
						var side proto.Side
						if externalPrice > currentPrice {
							side = proto.Side_SIDE_BUY
							shouldMove = "UP"
						} else {
							side = proto.Side_SIDE_SELL
							shouldMove = "DN"
						}

						// Now we call into the maths heavy function to find out
						// what price and size of the order we should place
						price, size, priceError := b.GetRealisticOrderDetails(externalPrice)

						if priceError != nil {
							b.log.Fatalf("Unable to get realistic order details for price steering: %v\n", priceError)
						}

						size = uint64(float64(size) * b.strategy.PriceSteerOrderScale)
						b.log.WithFields(log.Fields{
							"size":  size,
							"side":  side,
							"price": price,
						}).Debug("Submitting order")

						err = b.sendOrder(size,
							price,
							side,
							proto.Order_TIME_IN_FORCE_GTT,
							proto.Order_TYPE_LIMIT,
							"PriceSteeringOrder",
							int64(b.strategy.LimitOrderDistributionParams.GttLength))
					}
					b.log.WithFields(log.Fields{
						"currentPrice":  currentPrice,
						"externalPrice": externalPrice,
						"diff":          int(externalPrice) - int(b.currentPrice),
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

// GetRealisticOrderDetails uses magic to return a realistic order price and size
func (b *Bot) GetRealisticOrderDetails(externalPrice uint64) (price, size uint64, err error) {
	// Collect stuff from config that's common to all methods
	method := b.strategy.LimitOrderDistributionParams.Method

	sigma := b.strategy.TargetLNVol
	tgtTimeHorizonHours := b.strategy.LimitOrderDistributionParams.TgtTimeHorizonHours
	tgtTimeHorizonYrFrac := tgtTimeHorizonHours / 24.0 / 365.25
	numOrdersPerSec := b.strategy.MarketPriceSteeringRatePerSecond
	N := 3600 * numOrdersPerSec / tgtTimeHorizonHours
	tickSize := float64(1 / math.Pow(10, float64(b.market.DecimalPlaces)))
	delta := float64(b.strategy.LimitOrderDistributionParams.NumTicksFromMid) * tickSize

	// this converts something like BTCUSD 3912312345 (five decimal places)
	// to 39123.12345 float.
	M0 := float64(externalPrice) / math.Pow(10, float64(b.market.DecimalPlaces))

	var priceFloat float64
	size = 1
	switch method {
	case DiscreteThreeLevel:
		priceFloat, err = GeneratePriceUsingDiscreteThreeLevel(M0, delta, sigma, tgtTimeHorizonYrFrac, N)

		// we need to add back decimals
		price = uint64(math.Round(priceFloat * math.Pow(10, float64(b.market.DecimalPlaces))))
		return
	case CoinAndBinomial:
		return externalPrice, 1, nil
	default:
		err = fmt.Errorf("Method for generating price distributions not recognised: %w", err)
		return
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
