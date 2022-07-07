package normal

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/seed"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"
	commandspb "code.vegaprotocol.io/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
	"code.vegaprotocol.io/vegawallet/wallets"
	log "github.com/sirupsen/logrus"
)

// TODO: make thread safe
// Bot represents one Normal liquidity bot.
type Bot struct {
	config                 config.BotConfig
	seedConfig             *config.SeedConfig
	active                 bool
	log                    *log.Entry
	pricingEngine          PricingEngine
	settlementAssetID      string
	settlementAssetAddress string
	stopPosMgmt            chan bool
	stopPriceSteer         chan bool
	strategy               *Strategy
	market                 *vega.Market
	node                   DataNode

	balanceGeneral *num.Uint
	balanceMargin  *num.Uint
	balanceBond    *num.Uint

	walletClient     types.WalletClient
	walletPassphrase string
	walletPubKey     string // "58595a" ...

	proposalIDCh      chan string
	proposalEnactedCh chan string
	stakeLinkingCh    chan struct{}

	buyShape   []*vega.LiquidityOrder
	sellShape  []*vega.LiquidityOrder
	marketData *vega.MarketData

	currentPrice       *num.Uint
	staticMidPrice     *num.Uint
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
func New(botConf config.BotConfig, seedConf *config.SeedConfig, pe PricingEngine, wc types.WalletClient) (b *Bot, err error) {
	b = &Bot{
		config:     botConf,
		seedConfig: seedConf,
		log: log.WithFields(log.Fields{
			"bot":  botConf.Name,
			"node": botConf.Location,
		}),
		pricingEngine:       pe,
		walletClient:        wc,
		eventStreamLive:     false,
		positionStreamLive:  false,
		auctionOrdersPlaced: false,
		proposalIDCh:        make(chan string),
		proposalEnactedCh:   make(chan string),
		stakeLinkingCh:      make(chan struct{}),
	}

	b.strategy, err = validateStrategyConfig(botConf.StrategyDetails)
	if err != nil {
		err = fmt.Errorf("failed to read strategy details: %w", err)
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
		return fmt.Errorf("failed to setup wallet: %w", err)
	}

	b.node, err = node.NewDataNode(
		url.URL{Host: b.config.Location},
		time.Duration(b.config.ConnectTimeout)*time.Millisecond,
		time.Duration(b.config.CallTimeout)*time.Millisecond,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to Vega gRPC node: %w", err)
	}
	b.log.WithFields(log.Fields{
		"address": b.config.Location,
	}).Debug("Connected to Vega gRPC node")

	marketsResponse, err := b.node.Markets(&dataapipb.MarketsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get markets: %w", err)
	}
	b.market = nil

	if len(marketsResponse.Markets) == 0 {
		marketsResponse, err = b.createMarket()
		if err != nil {
			return fmt.Errorf("failed to create market: %w", err)
		}
	}

	for _, mkt := range marketsResponse.Markets {
		instrument := mkt.TradableInstrument.GetInstrument()
		if instrument != nil {
			md := instrument.Metadata
			base := ""
			quote := ""
			for _, tag := range md.Tags {
				parts := strings.Split(tag, ":")
				if len(parts) == 2 {
					if parts[0] == "quote" {
						quote = parts[1]
					}
					if parts[0] == "base" || parts[0] == "ticker" {
						base = parts[1]
					}
				}
			}
			if base == b.config.InstrumentBase && quote == b.config.InstrumentQuote {
				future := mkt.TradableInstrument.Instrument.GetFuture()
				if future != nil {
					b.settlementAssetID = future.SettlementAsset
					b.market = mkt
					break
				}
			}
		}
	}
	if b.market == nil {
		return fmt.Errorf("failed to find futures markets: base/ticker=%s, quote=%s", b.config.InstrumentBase, b.config.InstrumentQuote)
	}
	b.log.WithFields(log.Fields{
		"id":                b.market.Id,
		"base/ticker":       b.config.InstrumentBase,
		"quote":             b.config.InstrumentQuote,
		"settlementAssetID": b.settlementAssetID,
	}).Info("Fetched market info")

	// Use the settlementAssetID to lookup the settlement ethereum address
	assetResponse, err := b.node.AssetByID(&dataapipb.AssetByIDRequest{Id: b.settlementAssetID})
	if err != nil {
		return fmt.Errorf("unable to look up asset details for %s", b.settlementAssetID)
	}
	erc20 := assetResponse.Asset.Details.GetErc20()
	if erc20 != nil {
		b.settlementAssetAddress = erc20.ContractAddress
	} else {
		b.settlementAssetAddress = ""
	}

	b.balanceGeneral = num.Zero()
	b.balanceMargin = num.Zero()

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

func (b *Bot) seedOrders() error {
	// GTC SELL 400@1000
	if err := b.createSeedOrder(num.NewUint(1000), 400, vega.Side_SIDE_SELL, vega.Order_TIME_IN_FORCE_GTC); err != nil {
		return fmt.Errorf("failed to create seed order: %w", err)
	}

	time.Sleep(time.Second * 2)

	// GTC BUY 400@250
	if err := b.createSeedOrder(num.NewUint(250), 400, vega.Side_SIDE_BUY, vega.Order_TIME_IN_FORCE_GTC); err != nil {
		return fmt.Errorf("failed to create seed order: %w", err)
	}

	time.Sleep(time.Second * 2)

	for i := 0; !b.canPlaceOrders(); i++ {
		side := vega.Side_SIDE_BUY
		if i%2 == 0 {
			side = vega.Side_SIDE_SELL
		}

		if err := b.createSeedOrder(num.NewUint(500), 400, side, vega.Order_TIME_IN_FORCE_GFA); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		time.Sleep(time.Second * 2)
	}

	b.log.Debug("seed orders created")
	return nil
}

func (b *Bot) createSeedOrder(price *num.Uint, size uint64, side vega.Side, tif vega.Order_TimeInForce) error {
	b.log.WithFields(log.Fields{
		"size":  size,
		"side":  side,
		"price": price,
		"tif":   tif.String(),
	}).Debug("Submitting seed order")

	if err := b.submitOrder(
		size,
		price,
		side,
		tif,
		vega.Order_TYPE_LIMIT,
		"MarketCreation",
		int64(b.strategy.PosManagementFraction)); err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

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

// GetTraderDetails returns information relating to the trader.
func (b *Bot) GetTraderDetails() string {
	name := b.config.Name
	pubKey := b.walletPubKey
	settlementVegaAssetID := b.settlementAssetID
	settlementEthereumContractAddress := b.settlementAssetAddress

	return "{\"name\":\"" + name + "\",\"pubKey\":\"" + pubKey + "\",\"settlementVegaAssetID\":\"" +
		settlementVegaAssetID + "\",\"settlementEthereumContractAddress\":\"" +
		settlementEthereumContractAddress + "\"}"
}

func (b *Bot) createMarket() (*dataapipb.MarketsResponse, error) {
	if err := b.subscribeToStakeLinkingEvents(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to stake linking events: %w", err)
	}

	b.log.Debug("minting and staking tokens")

	seedSvc, err := seed.NewService(b.seedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create seed service: %w", err)
	}

	if err := seedSvc.SeedStakeDeposit(b.walletPubKey); err != nil {
		return nil, fmt.Errorf("failed to seed stake tokens: %w", err)
	}

	b.log.Debug("waiting for stake to propagate...")

	if err := b.waitForStakeLinking(); err != nil {
		return nil, fmt.Errorf("failed stake linking: %w", err)
	}

	b.log.Debug("successfully linked stake")

	if err := b.subscribeToProposalEvents(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to proposal events: %w", err)
	}

	b.log.Debug("sending new market proposal")

	if err := b.sendNewMarketProposal(); err != nil {
		return nil, fmt.Errorf("failed to send new market proposal: %w", err)
	}

	b.log.Debug("waiting for proposal ID...")

	proposalID, err := b.waitForProposalID()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for proposal ID: %w", err)
	}

	b.log.Debug("successfully sent new market proposal")
	b.log.Debug("sending votes for market proposal")

	if err = b.sendVote(b.walletPubKey, proposalID, true); err != nil {
		return nil, fmt.Errorf("failed to send vote: %w", err)
	}

	b.log.Debug("waiting for proposal to be enacted...")

	if err = b.waitForProposalEnacted(proposalID); err != nil {
		return nil, fmt.Errorf("failed to wait for proposal to be enacted: %w", err)
	}

	b.log.Debug("market proposal successfully enacted")

	marketsResponse, err := b.node.Markets(&dataapipb.MarketsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}

	if len(marketsResponse.Markets) != 0 {
		b.market = marketsResponse.Markets[0]
	}

	return marketsResponse, nil
}

func (b *Bot) canPlaceOrders() bool {
	return b.marketData != nil && b.marketData.MarketTradingMode == vega.Market_TRADING_MODE_CONTINUOUS
}

func (b *Bot) waitForStakeLinking() error {
	for {
		select {
		case <-b.stakeLinkingCh:
			return nil
		case <-time.NewTimer(time.Second * 20).C:
			return fmt.Errorf("timeout waiting for stake linking")
		}
	}
}

func (b *Bot) waitForProposalID() (string, error) {
	for {
		select {
		case id := <-b.proposalIDCh:
			b.log.WithFields(log.Fields{
				"proposalID": id,
			}).Info("Received proposal ID")
			return id, nil
		case <-time.NewTimer(time.Second * 15).C:
			return "", fmt.Errorf("timeout waiting for proposal ID")
		}
	}
}

func (b *Bot) waitForProposalEnacted(pID string) error {
	for {
		select {
		case id := <-b.proposalEnactedCh:
			if id == pID {
				b.log.WithFields(log.Fields{
					"proposalID": id,
				}).Info("Proposal was enacted")
				return nil
			}
		case <-time.NewTimer(time.Second * 25).C:
			return fmt.Errorf("timeout waiting for proposal to be enacted")
		}
	}
}

func mulFrac(n *num.Uint, x float64, precision float64) *num.Uint {
	val := num.NewUint(uint64(x * math.Pow(10, precision)))
	val.Mul(val, n)
	val.Div(val, num.NewUint(uint64(math.Pow(10, precision))))
	return val
}

func (b *Bot) sendLiquidityProvision(buys, sells []*vega.LiquidityOrder) error {
	// CommitmentAmount is the fractional commitment value * total collateral
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	commitment := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
		LiquidityProvisionSubmission: &commandspb.LiquidityProvisionSubmission{
			Fee:              b.config.StrategyDetails.Fee,
			MarketId:         b.market.Id,
			CommitmentAmount: commitment.String(),
			Buys:             buys,
			Sells:            sells,
		},
	}
	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}
	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission(%v): %w", cmd, err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionSubmission")
	return nil
}

func (b *Bot) sendLiquidityProvisionAmendment(buys, sells []*vega.LiquidityOrder) error {
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	commitment := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)

	if commitment == num.NewUint(0) {
		return b.sendLiquidityProvisionCancellation(balTotal)
	}

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
		LiquidityProvisionAmendment: &commandspb.LiquidityProvisionAmendment{
			MarketId:         b.market.Id,
			CommitmentAmount: commitment.String(),
			Fee:              b.config.StrategyDetails.Fee,
			Sells:            sells,
			Buys:             buys,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *Bot) sendLiquidityProvisionCancellation(balTotal *num.Uint) error {
	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionCancellation{
		LiquidityProvisionCancellation: &commandspb.LiquidityProvisionCancellation{
			MarketId: b.market.Id,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionCancellation: %w", err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         num.NewUint(0),
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
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
		err := b.sendLiquidityProvisionAmendment(b.buyShape, b.sellShape)
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
	if !b.canPlaceOrders() {
		// Only allow position management during continuous trading
		return
	}
	var shouldPlaceOrder bool

	// TODO: add calculations
	// posMarginCost := calculatePositionMarginCost(b.openVolume, b.currentPrice, nil)
	// maxCost := uint64((1.0-b.strategy.StakeFraction-b.strategy.OrdersFraction)*b.balanceGeneral.Float64()

	// if posMarginCost > maxCost {
	// 	if b.openVolume > 0 {
	// 		shouldSell = true
	// 	} else if b.openVolume < 0 {
	// 		shouldBuy = true
	// 	}
	// }
	var size uint64
	var side vega.Side
	if b.openVolume >= 0 && num.NewUint(uint64(b.openVolume)).GT(b.strategy.MaxLong.Get()) {
		shouldPlaceOrder = true
		size = mulFrac(num.NewUint(uint64(b.openVolume)), b.strategy.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_SELL
	} else if b.openVolume < 0 && num.NewUint(uint64(-b.openVolume)).GT(b.strategy.MaxShort.Get()) {
		shouldPlaceOrder = true
		size = mulFrac(num.NewUint(uint64(-b.openVolume)), b.strategy.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_BUY
	}

	if !shouldPlaceOrder {
		return
	}

	err := b.submitOrder(size, num.Zero(), side, vega.Order_TIME_IN_FORCE_IOC, vega.Order_TYPE_MARKET, "PosManagement", 0)
	if err != nil {
		b.log.WithFields(log.Fields{
			"error": err,
			"side":  side,
			"size":  size,
		}).Warning("Failed to place a position management order")
	}
}

func (b *Bot) signSubmitTx(
	submitTxReq *walletpb.SubmitTransactionRequest,
	blockData *vegaapipb.LastBlockHeightResponse,
) error {
	msg := "failed to sign+submit tx: %w"

	if blockData == nil {
		var err error
		blockData, err = b.node.LastBlockData()
		if err != nil {
			return fmt.Errorf(msg, fmt.Errorf("failed to get block height: %w", err))
		}
	}

	submitTxReq.Propagate = true

	_, err := b.walletClient.SignTx(submitTxReq)
	if err != nil {
		return fmt.Errorf(msg, fmt.Errorf("failed to sign tx: %w", err))
	}

	return nil
}

func (b *Bot) submitOrder(
	size uint64,
	price *num.Uint,
	side vega.Side,
	tif vega.Order_TimeInForce,
	orderType vega.Order_Type,
	reference string,
	secondsFromNow int64,
) error {
	cmd := &walletpb.SubmitTransactionRequest_OrderSubmission{
		OrderSubmission: &commandspb.OrderSubmission{
			MarketId:    b.market.Id,
			Price:       "", // added below
			Size:        size,
			Side:        side,
			TimeInForce: tif,
			ExpiresAt:   0, // added below
			Type:        orderType,
			Reference:   reference,
			PeggedOrder: nil,
		},
	}
	if tif == vega.Order_TIME_IN_FORCE_GTT {
		cmd.OrderSubmission.ExpiresAt = time.Now().UnixNano() + (secondsFromNow * 1000000000)
	}
	if orderType != vega.Order_TYPE_MARKET {
		cmd.OrderSubmission.Price = price.String()
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}
	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit OrderSubmission: %w", err)
	}
	return nil
}

func (b *Bot) checkInitialMargin() error {
	// Turn the shapes into a set of orders scaled by commitment
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	obligation := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)
	buyOrders := b.calculateOrderSizes(b.market.Id, b.walletPubKey, obligation, b.buyShape)
	sellOrders := b.calculateOrderSizes(b.market.Id, b.walletPubKey, obligation, b.sellShape)

	buyRisk := 0.01
	sellRisk := 0.01

	buyCost := b.calculateMarginCost(buyRisk, buyOrders)
	sellCost := b.calculateMarginCost(sellRisk, sellOrders)

	shapeMarginCost := num.Max(buyCost, sellCost)

	avail := mulFrac(b.balanceGeneral, b.strategy.OrdersFraction, 15)

	if avail.LT(shapeMarginCost) {
		var missingPercent string
		if avail.EQUint64(0) {
			missingPercent = "Inf"
		} else {
			x := num.UintChain(shapeMarginCost).Sub(avail).Mul(num.NewUint(100)).Div(avail).Get()
			missingPercent = fmt.Sprintf("%v%%", x)
		}
		b.log.WithFields(log.Fields{
			"available":      avail,
			"cost":           shapeMarginCost,
			"missing":        num.Zero().Sub(avail, shapeMarginCost),
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
		b.log.WithFields(log.Fields{"error": err.Error()}).
			Debug("Stopping position management as we could not get initial values")
		return err
	}

	if !b.eventStreamLive {
		if err = b.subscribeToMarketEvents(); err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).
				Debug("Unable to subscribe to market event bus feeds")
			return err
		}

		if err = b.subscribeToAccountEvents(); err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).
				Debug("Unable to subscribe to account event bus feeds")
			return err
		}
	}

	if !b.positionStreamLive {
		err = b.subscribePositions()
		if err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).
				Debug("Unable to subscribe to positions event bus feeds")
			return err
		}
	}
	return nil
}

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *Bot) placeAuctionOrders() {
	// Check we have not placed them already
	if b.auctionOrdersPlaced {
		return
	}
	// Check we have a currentPrice we can use
	if b.currentPrice.EQUint64(0) {
		return
	}

	// Place the random orders split into
	totalVolume := num.Zero()
	rand.Seed(time.Now().UnixNano())
	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		remaining := num.Zero().Sub(b.config.StrategyDetails.AuctionVolume.Get(), totalVolume)
		size := num.Min(num.UintChain(b.config.StrategyDetails.AuctionVolume.Get()).Div(num.NewUint(10)).Add(num.NewUint(1)).Get(), remaining)
		// #nosec G404
		price := num.Zero().Add(b.currentPrice, num.NewUint(uint64(rand.Int63n(100)-50)))
		side := vega.Side_SIDE_BUY
		// #nosec G404
		if rand.Intn(2) == 0 {
			side = vega.Side_SIDE_SELL
		}
		err := b.submitOrder(size.Uint64(), price, side, vega.Order_TIME_IN_FORCE_GTT, vega.Order_TYPE_LIMIT, "AuctionOrder", 330)
		if err == nil {
			totalVolume = num.Zero().Add(totalVolume, size)
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
				}
				firstTime = false
			}

			// Only update liquidity and position if we are not in auction
			if b.canPlaceOrders() {
				b.auctionOrdersPlaced = false
				b.checkForShapeChange()
				b.checkPositionManagement()
			} else {
				b.placeAuctionOrders()
			}

			// If we have lost the incoming streams we should attempt to reconnect
			for !b.positionStreamLive || !b.eventStreamLive {
				b.log.WithFields(log.Fields{
					"eventStreamLive":    b.eventStreamLive,
					"positionStreamLive": b.positionStreamLive,
				}).Debug("Stream info")
				err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
				if err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).
						Debug("Stopping bot position management")
					b.active = false
					return
				}

				err = b.initialiseData()
				if err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Debug("Failed to initialise data")
					continue
				}
			}

			err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
			if err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).
					Debug("Stopping bot position management")
				b.active = false
				return
			}
		}
	}
}

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice).
func (b *Bot) calculateOrderSizes(marketID, partyID string, obligation *num.Uint, liquidityOrders []*vega.LiquidityOrder) []*vega.Order {
	orders := make([]*vega.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	totalProportion := num.Zero()
	for _, order := range liquidityOrders {
		totalProportion.Add(totalProportion, num.NewUint(uint64(order.Proportion)))
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		size := num.UintChain(obligation).Mul(num.NewUint(uint64(lo.Proportion))).Mul(num.NewUint(10)).Div(totalProportion).Div(b.currentPrice).Get()
		peggedOrder := vega.PeggedOrder{
			Reference: lo.Reference,
			Offset:    lo.Offset,
		}

		order := vega.Order{
			MarketId:    marketID,
			PartyId:     partyID,
			Side:        vega.Side_SIDE_BUY,
			Remaining:   size.Uint64(),
			Size:        size.Uint64(),
			TimeInForce: vega.Order_TIME_IN_FORCE_GTC,
			Type:        vega.Order_TYPE_LIMIT,
			PeggedOrder: &peggedOrder,
		}
		orders = append(orders, &order)
	}
	return orders
}

// calculateMarginCost estimates the margin cost of the set of orders.
func (b *Bot) calculateMarginCost(risk float64, orders []*vega.Order) *num.Uint {
	// totalMargin := num.Zero()
	margins := make([]*num.Uint, len(orders))
	for i, order := range orders {
		if order.Side == vega.Side_SIDE_BUY {
			margins[i] = num.NewUint(1 + order.Size)
		} else {
			margins[i] = num.NewUint(order.Size)
		}
	}
	totalMargin := num.UintChain(num.NewUint(0)).Add(margins...).Mul(b.currentPrice).Get()
	totalMargin = mulFrac(totalMargin, risk, 15)
	return totalMargin
}

func (b *Bot) runPriceSteering() {
	if !b.canPlaceOrders() {
		if err := b.seedOrders(); err != nil {
			b.log.WithFields(
				log.Fields{
					"error": err.Error(),
				}).Error("Failed to seed orders")
		}
	}

	var externalPrice *num.Uint
	var currentPrice *num.Uint
	var err error
	var externalPriceResponse ppservice.PriceResponse

	ppcfg := ppconfig.PriceConfig{
		Base:   b.config.InstrumentBase,
		Quote:  b.config.InstrumentQuote,
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
			canPlaceOrders := b.canPlaceOrders() // redundant?
			if b.strategy.PriceSteerOrderScale > 0 && canPlaceOrders {
				externalPriceResponse, err = b.pricingEngine.GetPrice(ppcfg)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get external price")
					externalPrice = num.Zero()
					currentPrice = num.Zero()
				} else {
					externalPrice = num.UintChain(num.NewUint(uint64(externalPriceResponse.Price))).Mul(num.NewUint(uint64(math.Pow10(int(b.market.DecimalPlaces))))).Get()
					currentPrice = b.staticMidPrice
				}

				if err == nil && currentPrice != nil && externalPrice != nil && !externalPrice.IsZero() {
					shouldMove := "no"
					// We only want to steer the price if the external and market price
					// are greater than a certain percentage apart
					var currentDiff *num.Uint

					// currentDiff = 100 * Abs(currentPrice - externalPrice) / externalPrice
					if currentPrice.GT(externalPrice) {
						currentDiff = num.Zero().Sub(currentPrice, externalPrice)
					} else {
						currentDiff = num.Zero().Sub(externalPrice, currentPrice)
					}
					currentDiff = num.UintChain(currentDiff).Mul(num.NewUint(100)).Div(externalPrice).Get()
					if currentDiff.GT(num.NewUint(uint64(100.0 * b.strategy.MinPriceSteerFraction))) {
						var side vega.Side
						if externalPrice.GT(currentPrice) {
							side = vega.Side_SIDE_BUY
							shouldMove = "UP"
						} else {
							side = vega.Side_SIDE_SELL
							shouldMove = "DN"
						}

						// Now we call into the maths heavy function to find out
						// what price and size of the order we should place
						price, size, priceError := b.GetRealisticOrderDetails(externalPrice)

						if priceError != nil {
							b.log.WithFields(log.Fields{"error": priceError.Error()}).
								Fatal("Unable to get realistic order details for price steering")
						}

						size = mulFrac(size, b.strategy.PriceSteerOrderScale, 15)
						b.log.WithFields(log.Fields{
							"size":  size,
							"side":  side,
							"price": price,
						}).Debug("Submitting order")

						err = b.submitOrder(
							size.Uint64(),
							price,
							side,
							vega.Order_TIME_IN_FORCE_GTT,
							vega.Order_TYPE_LIMIT,
							"PriceSteeringOrder",
							int64(b.strategy.LimitOrderDistributionParams.GttLength))
					}
					b.log.WithFields(log.Fields{
						"currentPrice":  currentPrice,
						"externalPrice": externalPrice,
						"diff":          num.Zero().Sub(externalPrice, b.currentPrice),
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
			} else {
				b.log.WithFields(log.Fields{
					"PriceSteerOrderScale": b.strategy.PriceSteerOrderScale,
					"canPlaceOrders":       canPlaceOrders,
				}).Debug("Price steering: Cannot place orders")
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

// GetRealisticOrderDetails uses magic to return a realistic order price and size.
func (b *Bot) GetRealisticOrderDetails(externalPrice *num.Uint) (price, size *num.Uint, err error) {
	// Collect stuff from config that's common to all methods
	method := b.strategy.LimitOrderDistributionParams.Method

	sigma := b.strategy.TargetLNVol
	tgtTimeHorizonHours := b.strategy.LimitOrderDistributionParams.TgtTimeHorizonHours
	tgtTimeHorizonYrFrac := tgtTimeHorizonHours / 24.0 / 365.25
	numOrdersPerSec := b.strategy.MarketPriceSteeringRatePerSecond
	N := 3600 * numOrdersPerSec / tgtTimeHorizonHours
	tickSize := 1.0 / math.Pow(10, float64(b.market.DecimalPlaces))
	delta := float64(b.strategy.LimitOrderDistributionParams.NumTicksFromMid) * tickSize

	// this converts something like BTCUSD 3912312345 (five decimal places)
	// to 39123.12345 float.
	M0 := num.Zero().Div(externalPrice, num.NewUint(uint64(math.Pow(10, float64(b.market.DecimalPlaces)))))

	var priceFloat float64
	size = num.NewUint(1)
	switch method {
	case DiscreteThreeLevel:
		priceFloat, err = GeneratePriceUsingDiscreteThreeLevel(M0.Float64(), delta, sigma, tgtTimeHorizonYrFrac, N)

		// we need to add back decimals
		price = num.NewUint(uint64(math.Round(priceFloat * math.Pow(10, float64(b.market.DecimalPlaces)))))
		return
	case CoinAndBinomial:
		price = externalPrice
		return
	default:
		err = fmt.Errorf("Method for generating price distributions not recognised: %w", err)
		return
	}
}

func (b *Bot) setupWallet() error {
	b.walletPassphrase = "123"

	if err := b.walletClient.LoginWallet(b.config.Name, b.walletPassphrase); err != nil {
		if strings.Contains(err.Error(), wallets.ErrWalletDoesNotExists.Error()) {
			if err = b.walletClient.CreateWallet(b.config.Name, b.walletPassphrase); err != nil {
				return fmt.Errorf("failed to create wallet: %w", err)
			}
			b.log.Debug("Created and logged into wallet")
		} else {
			return fmt.Errorf("failed to log into wallet: %w", err)
		}
	}

	b.log.Debug("Logged into wallet")

	if b.walletPubKey == "" {
		publicKeys, err := b.walletClient.ListPublicKeys()
		if err != nil {
			return fmt.Errorf("failed to list public keys: %w", err)
		}

		if len(publicKeys) == 0 {
			key, err := b.walletClient.GenerateKeyPair(b.walletPassphrase, []types.Meta{})
			if err != nil {
				return fmt.Errorf("failed to generate keypair: %w", err)
			}
			b.walletPubKey = key.Pub
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Created keypair")
		} else {
			b.walletPubKey = publicKeys[0]
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Using existing keypair")
		}
	}

	b.log = b.log.WithFields(log.Fields{"pubkey": b.walletPubKey})

	return nil
}
