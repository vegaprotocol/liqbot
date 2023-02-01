package market

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	itypes "code.vegaprotocol.io/liqbot/types"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/shared/libs/wallet"
	"code.vegaprotocol.io/vega/logging"
	v12 "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	v1 "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	oraclesv1 "code.vegaprotocol.io/vega/protos/vega/data/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type Service struct {
	pricingEngine itypes.PricingEngine
	marketStream  marketStream
	node          dataNode
	wallet        wallet.WalletV2
	account       accountService
	config        config.BotConfig
	log           *logging.Logger

	marketDecimalPlaces uint64
	pubKey              string
	marketID            string
	vegaAssetID         string
}

func NewService(
	log *logging.Logger,
	node dataNode,
	wallet wallet.WalletV2,
	pe itypes.PricingEngine,
	account accountService,
	marketStream marketStream,
	config config.BotConfig,
	pubKey,
	vegaAssetID string,
) *Service {
	return &Service{
		marketStream:  marketStream,
		node:          node,
		wallet:        wallet,
		pricingEngine: pe,
		account:       account,
		config:        config,
		pubKey:        pubKey,
		vegaAssetID:   vegaAssetID,
		log:           log.Named("MarketService"),
	}
}

func (m *Service) Market() cache.MarketData {
	return m.marketStream.Store().Market()
}

// SetupMarket creates a market if it doesn't exist, provides liquidity and gets the market into continuous trading mode.
func (m *Service) SetupMarket(ctx context.Context) (*vega.Market, error) {
	market, err := m.ProvideMarket(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to provide market: %w", err)
	}

	m.log.Info("Starting market service")
	if err := m.marketStream.Subscribe(ctx, market.Id); err != nil {
		return nil, fmt.Errorf("failed to subscribe to market stream: %w", err)
	}
	m.marketID = market.Id

	if err = m.ProvideLiquidity(ctx); err != nil {
		return nil, fmt.Errorf("failed to provide liquidity: %w", err)
	}

	if !m.CanPlaceOrders() {
		if err = m.SeedOrders(ctx); err != nil {
			return nil, fmt.Errorf("failed to seed orders: %w", err)
		}
	}

	return market, nil
}

func (m *Service) ProvideMarket(ctx context.Context) (*vega.Market, error) {
	market, err := m.findMarket(ctx)
	if err == nil {
		m.log.With(logging.Market(market)).Info("Found market")
		return market, nil
	}

	m.log.Info("Failed to find market, creating it")

	if err = m.CreateMarket(ctx); err != nil {
		return nil, fmt.Errorf("failed to create market: %w", err)
	}

	market, err = m.findMarket(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find market after creation: %w", err)
	}

	return market, nil
}

func (m *Service) findMarket(ctx context.Context) (*vega.Market, error) {
	marketsResponse, err := m.node.Markets(ctx, &v12.ListMarketsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}

	for _, mkt := range marketsResponse {
		instrument := mkt.TradableInstrument.GetInstrument()
		if instrument == nil {
			continue
		}

		future := instrument.GetFuture()
		if future == nil {
			continue
		}

		base := ""
		quote := ""

		for _, tag := range instrument.Metadata.Tags {
			parts := strings.Split(tag, ":")
			if len(parts) != 2 {
				continue
			}
			if parts[0] == "quote" {
				quote = parts[1]
			}
			if parts[0] == "base" || parts[0] == "ticker" {
				base = parts[1]
			}
		}

		if base != m.config.InstrumentBase || quote != m.config.InstrumentQuote {
			continue
		}

		m.log = m.log.With(logging.MarketID(mkt.Id))
		m.marketDecimalPlaces = mkt.DecimalPlaces

		return mkt, nil
	}

	return nil, fmt.Errorf("failed to find futures markets: base/ticker=%s, quote=%s", m.config.InstrumentBase, m.config.InstrumentQuote)
}

func (m *Service) CreateMarket(ctx context.Context) error {
	m.log.Info("Minting, staking and depositing tokens")

	stakeAmount := m.config.StrategyDetails.StakeAmount.Get()

	m.log.With(
		logging.String("amount", stakeAmount.String()),
		logging.String("asset", m.config.SettlementAssetID),
		logging.String("name", m.config.Name),
	).Info("Ensuring balance for market creation")

	// TODO: this is probably unnecessary
	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.General, stakeAmount, m.marketDecimalPlaces, 1, "MarketCreation"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	m.log.With(
		logging.String("amount", stakeAmount.String()),
		logging.String("asset", m.config.SettlementAssetID),
		logging.String("name", m.config.Name),
	).Info("Balance ensured")

	m.log.With(
		logging.String("amount", stakeAmount.String()),
		logging.String("asset", m.vegaAssetID),
		logging.String("name", m.config.Name),
	).Info("Ensuring stake for market creation")

	if err := m.account.EnsureStake(ctx, m.config.Name, m.wallet.PublicKey(), m.vegaAssetID, stakeAmount, "MarketCreation"); err != nil {
		return fmt.Errorf("failed to ensure stake: %w", err)
	}

	m.log.With(
		logging.String("amount", stakeAmount.String()),
		logging.String("asset", m.vegaAssetID),
		logging.String("name", m.config.Name),
	).Info("Successfully linked stake")

	m.log.Info("Sending new market proposal...")

	if err := m.sendNewMarketProposal(ctx); err != nil {
		return fmt.Errorf("failed to send new market proposal: %w", err)
	}

	m.log.Debug("Waiting for proposal ID...")

	proposalID, err := m.marketStream.waitForProposalID()
	if err != nil {
		return fmt.Errorf("failed to wait for proposal ID: %w", err)
	}

	m.log.Debug("Successfully sent new market proposal")
	m.log.Debug("Sending votes for market proposal")

	if err = m.sendVote(ctx, proposalID, true); err != nil {
		return fmt.Errorf("failed to send vote: %w", err)
	}

	m.log.Debug("Waiting for proposal to be enacted...")

	if err = m.marketStream.waitForProposalEnacted(proposalID); err != nil {
		return fmt.Errorf("failed to wait for proposal to be enacted: %w", err)
	}

	m.log.Debug("Market proposal successfully enacted")

	return nil
}

func (m *Service) sendNewMarketProposal(ctx context.Context) error {
	cmd := &walletpb.SubmitTransactionRequest_ProposalSubmission{
		ProposalSubmission: m.getExampleMarketProposal(),
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: cmd,
	}

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	return nil
}

func (m *Service) sendVote(ctx context.Context, proposalId string, vote bool) error {
	value := vega.Vote_VALUE_NO
	if vote {
		value = vega.Vote_VALUE_YES
	}

	cmd := &walletpb.SubmitTransactionRequest_VoteSubmission{
		VoteSubmission: &v1.VoteSubmission{
			ProposalId: proposalId,
			Value:      value,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: cmd,
	}

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit Vote Submission: %w", err)
	}

	return nil
}

func (m *Service) ProvideLiquidity(ctx context.Context) error {
	commitment, err := m.GetRequiredCommitment()
	if err != nil {
		return fmt.Errorf("failed to get required commitment: %w", err)
	}

	if commitment.IsZero() {
		return nil
	}

	if err = m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.GeneralAndBond, commitment, m.marketDecimalPlaces, 2, "MarketCreation"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	// We always cache off with longening shapes
	buyShape, sellShape, _ := m.GetShape()
	// At the cache of each loop, wait for positive general account balance. This is in case the network has
	// been restarted.
	if err := m.CheckInitialMargin(ctx, buyShape, sellShape); err != nil {
		return fmt.Errorf("failed initial margin check: %w", err)
	}

	// Submit LP order to market.
	if err = m.SendLiquidityProvision(ctx, commitment, buyShape, sellShape); err != nil {
		return fmt.Errorf("failed to send liquidity provision order: %w", err)
	}

	return nil
}

func (m *Service) CheckInitialMargin(ctx context.Context, buyShape, sellShape []*vega.LiquidityOrder) error {
	// Turn the shapes into a set of orders scaled by commitment
	obligation := cache.GeneralAndBond(m.account.Balance(ctx, m.config.SettlementAssetID))
	buyOrders := m.calculateOrderSizes(obligation, buyShape)
	sellOrders := m.calculateOrderSizes(obligation, sellShape)

	buyRisk := 0.01
	sellRisk := 0.01

	buyCost := m.calculateMarginCost(buyRisk, buyOrders)
	sellCost := m.calculateMarginCost(sellRisk, sellOrders)

	shapeMarginCost := num.Max(buyCost, sellCost)
	avail := num.MulFrac(cache.General(m.account.Balance(ctx, m.config.SettlementAssetID)), m.config.StrategyDetails.OrdersFraction, 15)

	if !avail.LT(shapeMarginCost) {
		return nil
	}

	missingPercent := "Inf"

	if !avail.IsZero() {
		x := num.UintChain(shapeMarginCost).Sub(avail).Mul(num.NewUint(100)).Div(avail).Get()
		missingPercent = fmt.Sprintf("%v%%", x)
	}

	m.log.With(
		logging.String("available", avail.String()),
		logging.String("cost", shapeMarginCost.String()),
		logging.String("missing", num.Zero().Sub(avail, shapeMarginCost).String()),
		logging.String("missingPercent", missingPercent),
	).Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")

	return errors.New("not enough collateral")
}

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice).
func (m *Service) calculateOrderSizes(obligation *num.Uint, liquidityOrders []*vega.LiquidityOrder) []*vega.Order {
	orders := make([]*vega.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	totalProportion := num.Zero()
	for _, order := range liquidityOrders {
		totalProportion.Add(totalProportion, num.NewUint(uint64(order.Proportion)))
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		size := num.UintChain(obligation.Clone()).
			Mul(num.NewUint(uint64(lo.Proportion))).
			Mul(num.NewUint(10)).
			Div(totalProportion).Div(m.Market().MarkPrice()).Get()
		peggedOrder := vega.PeggedOrder{
			Reference: lo.Reference,
			Offset:    lo.Offset,
		}

		order := vega.Order{
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
func (m *Service) calculateMarginCost(risk float64, orders []*vega.Order) *num.Uint {
	margins := make([]*num.Uint, len(orders))

	for i, order := range orders {
		if order.Side == vega.Side_SIDE_BUY {
			margins[i] = num.NewUint(1 + order.Size)
		} else {
			margins[i] = num.NewUint(order.Size)
		}
	}

	totalMargin := num.UintChain(num.Zero()).Add(margins...).Mul(m.Market().MarkPrice()).Get()
	return num.MulFrac(totalMargin, risk, 15)
}

func (m *Service) GetShape() ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string) {
	// We always cache off with longening shapes
	shape := "longening"
	buyShape := m.config.StrategyDetails.LongeningShape.Buys.ToVegaLiquidityOrders()
	sellShape := m.config.StrategyDetails.LongeningShape.Sells.ToVegaLiquidityOrders()

	if m.Market().OpenVolume() > 0 {
		shape = "shortening"
		buyShape = m.config.StrategyDetails.ShorteningShape.Buys.ToVegaLiquidityOrders()
		sellShape = m.config.StrategyDetails.ShorteningShape.Sells.ToVegaLiquidityOrders()
	}

	return buyShape, sellShape, shape
}

func (m *Service) CheckPosition() (uint64, vega.Side, bool) {
	size := uint64(0)
	side := vega.Side_SIDE_UNSPECIFIED
	shouldPlace := true
	openVolume := m.Market().OpenVolume()

	if openVolume >= 0 && num.NewUint(uint64(openVolume)).GT(m.config.StrategyDetails.MaxLong.Get()) {
		size = num.MulFrac(num.NewUint(uint64(openVolume)), m.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_SELL
	} else if openVolume < 0 && num.NewUint(uint64(-openVolume)).GT(m.config.StrategyDetails.MaxShort.Get()) {
		size = num.MulFrac(num.NewUint(uint64(-openVolume)), m.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_BUY
	} else {
		shouldPlace = false
	}

	return size, side, shouldPlace
}

func (m *Service) SendLiquidityProvision(ctx context.Context, commitment *num.Uint, buys, sells []*vega.LiquidityOrder) error {
	ref := m.config.Name + "-" + commitment.String()
	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
			LiquidityProvisionSubmission: &commandspb.LiquidityProvisionSubmission{
				MarketId:         m.marketID,
				CommitmentAmount: commitment.String(),
				Fee:              m.config.StrategyDetails.Fee,
				Sells:            sells,
				Buys:             buys,
				Reference:        ref,
			},
		},
	}

	m.log.With(logging.String("commitment", commitment.String())).Debug("Submitting LiquidityProvisionSubmission...")

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission: %w", err)
	}

	if err := m.marketStream.waitForLiquidityProvision(ctx, ref); err != nil {
		return fmt.Errorf("failed to wait for liquidity provision to be active: %w", err)
	}

	m.log.With(logging.String("commitment", commitment.String())).Debug("Submitted LiquidityProvisionSubmission")

	return nil
}

// call this if the position flips.
func (m *Service) SendLiquidityProvisionAmendment(ctx context.Context, commitment *num.Uint, buys, sells []*vega.LiquidityOrder) error {
	var commitmentAmount string
	if commitment != nil {
		if commitment.IsZero() {
			return m.SendLiquidityProvisionCancellation(ctx)
		}
		commitmentAmount = commitment.String()
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
			LiquidityProvisionAmendment: &commandspb.LiquidityProvisionAmendment{
				MarketId:         m.marketID,
				CommitmentAmount: commitmentAmount,
				Fee:              m.config.StrategyDetails.Fee,
				Sells:            sells,
				Buys:             buys,
			},
		},
	}

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}

	m.log.With(logging.String("commitment", commitmentAmount)).Debug("Submitted LiquidityProvisionAmendment")
	return nil
}

func (m *Service) EnsureCommitmentAmount(ctx context.Context) error {
	requiredCommitment, err := m.GetRequiredCommitment()
	if err != nil {
		return fmt.Errorf("failed to get new commitment amount: %w", err)
	}

	if requiredCommitment.IsZero() {
		return nil
	}

	m.log.With(logging.String("newCommitment", requiredCommitment.String())).Debug("Supplied stake is less than target stake, increasing commitment amount...")

	if err = m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.GeneralAndBond, requiredCommitment, m.marketDecimalPlaces, 2, "MarketService"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	buys, sells, _ := m.GetShape()

	m.log.With(logging.String("newCommitment", requiredCommitment.String())).Debug("Sending new commitment amount...")

	if err = m.SendLiquidityProvisionAmendment(ctx, requiredCommitment, buys, sells); err != nil {
		return fmt.Errorf("failed to update commitment amount: %w", err)
	}

	return nil
}

func (m *Service) SendLiquidityProvisionCancellation(ctx context.Context) error {
	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionCancellation{
			LiquidityProvisionCancellation: &commandspb.LiquidityProvisionCancellation{
				MarketId: m.marketID,
			},
		},
	}

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionCancellation: %w", err)
	}

	m.log.With(logging.String("commitment", "0")).Debug("Submitted LiquidityProvisionCancellation")

	return nil
}

func (m *Service) GetRequiredCommitment() (*num.Uint, error) {
	suppliedStake := m.Market().SuppliedStake().Clone()
	targetStake := m.Market().TargetStake().Clone()

	if targetStake.IsZero() {
		var err error
		targetStake, err = num.ConvertUint256(m.config.StrategyDetails.CommitmentAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to convert commitment amount: %w", err)
		}
	}

	m.log.With(
		logging.String("suppliedStake", suppliedStake.String()),
		logging.String("targetStake", targetStake.String()),
	).Debug("Checking for required commitment")

	dx := suppliedStake.Int().Sub(targetStake.Int())

	if dx.IsPositive() {
		return num.Zero(), nil
	}

	return num.Zero().Add(targetStake, dx.Uint()), nil
}

func (m *Service) CanPlaceOrders() bool {
	return m.Market().TradingMode() == vega.Market_TRADING_MODE_CONTINUOUS
}

// TODO: make retryable.
func (m *Service) SubmitOrder(ctx context.Context, order *vega.Order, from string, secondsFromNow int64) error {
	cmd := &walletpb.SubmitTransactionRequest_OrderSubmission{
		OrderSubmission: &commandspb.OrderSubmission{
			MarketId:    order.MarketId,
			Price:       "", // added below
			Size:        order.Size,
			Side:        order.Side,
			TimeInForce: order.TimeInForce,
			ExpiresAt:   0, // added below
			Type:        order.Type,
			Reference:   order.Reference,
			PeggedOrder: nil,
		},
	}

	if order.TimeInForce == vega.Order_TIME_IN_FORCE_GTT {
		cmd.OrderSubmission.ExpiresAt = time.Now().UnixNano() + (secondsFromNow * 1000000000)
	}

	if order.Type != vega.Order_TYPE_MARKET {
		cmd.OrderSubmission.Price = order.Price
	}

	m.log.With(
		logging.Int64("expiresAt", cmd.OrderSubmission.ExpiresAt),
		logging.String("types", order.Type.String()),
		logging.String("reference", order.Reference),
		logging.Uint64("size", order.Size),
		logging.String("side", order.Side.String()),
		logging.String("price", order.Price),
		logging.String("tif", order.TimeInForce.String()),
	).Debugf("%s: Submitting order", from)

	submitTxReq := &walletpb.SubmitTransactionRequest{
		Command: cmd,
	}

	if _, err := m.wallet.SendTransaction(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit OrderSubmission: %w", err)
	}

	return nil
}

func (m *Service) SeedOrders(ctx context.Context) error {
	externalPrice, err := m.GetExternalPrice()
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	orders, totalCost := m.createSeedAuctionOrders(externalPrice.Clone())
	m.log.With(
		logging.String("externalPrice", externalPrice.String()),
		logging.String("totalCost", totalCost.String()),
		logging.String("balance.General", cache.General(m.account.Balance(ctx, m.config.SettlementAssetID)).String()),
	).Debug("Seeding auction orders")

	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.General, totalCost, m.marketDecimalPlaces, 2, "MarketService"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	for _, order := range orders {
		if err = m.SubmitOrder(ctx, order, "MarketService", int64(m.config.StrategyDetails.PosManagementFraction)); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		if m.CanPlaceOrders() {
			m.log.Debug("Trading mode is continuous")
			return nil
		}

		time.Sleep(time.Second * 2)
	}

	return fmt.Errorf("seeding orders did not end the auction")
}

func (m *Service) createSeedAuctionOrders(externalPrice *num.Uint) ([]*vega.Order, *num.Uint) {
	tif := vega.Order_TIME_IN_FORCE_GTC
	count := m.config.StrategyDetails.SeedOrderCount
	orders := make([]*vega.Order, count)
	totalCost := num.NewUint(0)
	size := m.config.StrategyDetails.SeedOrderSize

	for i := 0; i < count; i++ {
		side := vega.Side_SIDE_BUY
		if i%2 == 0 {
			side = vega.Side_SIDE_SELL
		}

		price := externalPrice.Clone()

		switch i {
		case 0:
			price = num.UintChain(price).Mul(num.NewUint(105)).Div(num.NewUint(100)).Get()
		case 1:
			price = num.UintChain(price).Mul(num.NewUint(95)).Div(num.NewUint(100)).Get()
		}

		totalCost.Add(totalCost, num.Zero().Mul(price.Clone(), num.NewUint(size)))

		orders[i] = &vega.Order{
			MarketId:    m.marketID,
			Size:        size,
			Price:       price.String(),
			Side:        side,
			TimeInForce: tif,
			Type:        vega.Order_TYPE_LIMIT,
			Reference:   "AuctionOrder",
		}
	}

	return orders, totalCost
}

func (m *Service) GetExternalPrice() (*num.Uint, error) {
	externalPriceResponse, err := m.pricingEngine.GetPrice(ppconfig.PriceConfig{
		Base:   m.config.InstrumentBase,
		Quote:  m.config.InstrumentQuote,
		Wander: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get external price: %w", err)
	}

	if externalPriceResponse.Price <= 0 {
		return nil, fmt.Errorf("external price is zero")
	}

	externalPrice := externalPriceResponse.Price * math.Pow(10, float64(m.marketDecimalPlaces))
	externalPriceNum := num.NewUint(uint64(externalPrice))
	return externalPriceNum, nil
}

func (m *Service) getExampleMarketProposal() *v1.ProposalSubmission {
	return &v1.ProposalSubmission{
		Rationale: &vega.ProposalRationale{
			Title:       "Example Market",
			Description: "some description",
		},
		Reference: "ProposalReference",
		Terms: &vega.ProposalTerms{
			ClosingTimestamp:   secondsFromNowInSecs(15),
			EnactmentTimestamp: secondsFromNowInSecs(15),
			Change: &vega.ProposalTerms_NewMarket{
				NewMarket: m.getExampleMarket(),
			},
		},
	}
}

func (m *Service) getExampleMarket() *vega.NewMarket {
	return &vega.NewMarket{
		Changes: &vega.NewMarketConfiguration{
			Instrument: &vega.InstrumentConfiguration{
				Code:    fmt.Sprintf("CRYPTO:%s%s/NOV22", m.config.InstrumentBase, m.config.InstrumentQuote),
				Name:    fmt.Sprintf("NOV 2022 %s vs %s future", m.config.InstrumentBase, m.config.InstrumentQuote),
				Product: m.getExampleProduct(),
			},
			DecimalPlaces: 5,
			Metadata:      []string{"base:" + m.config.InstrumentBase, "quote:" + m.config.InstrumentQuote, "class:fx/crypto", "monthly", "sector:crypto"},
			RiskParameters: &vega.NewMarketConfiguration_Simple{
				Simple: &vega.SimpleModelParams{
					FactorLong:           0.15,
					FactorShort:          0.25,
					MaxMoveUp:            10,
					MinMoveDown:          -5,
					ProbabilityOfTrading: 0.1,
				},
			},
			LpPriceRange: "25",
		},
	}
}

func (m *Service) getExampleProduct() *vega.InstrumentConfiguration_Future {
	return &vega.InstrumentConfiguration_Future{
		Future: &vega.FutureProduct{
			SettlementAsset: m.config.SettlementAssetID,
			QuoteName:       fmt.Sprintf("%s%s", m.config.InstrumentBase, m.config.InstrumentQuote),
			DataSourceSpecForSettlementData: &vega.DataSourceDefinition{
				SourceType: &vega.DataSourceDefinition_External{
					External: &vega.DataSourceDefinitionExternal{
						SourceType: &vega.DataSourceDefinitionExternal_Oracle{
							Oracle: &vega.DataSourceSpecConfiguration{
								Filters: []*oraclesv1.Filter{
									{
										Key: &oraclesv1.PropertyKey{
											Name: "prices.ETH.value",
											Type: oraclesv1.PropertyKey_TYPE_INTEGER,
										},
										Conditions: []*oraclesv1.Condition{},
									},
								},
								Signers: []*oraclesv1.Signer{
									{
										Signer: &oraclesv1.Signer_PubKey{
											PubKey: &oraclesv1.PubKey{
												Key: m.pubKey,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			DataSourceSpecForTradingTermination: &vega.DataSourceDefinition{
				SourceType: &vega.DataSourceDefinition_External{
					External: &vega.DataSourceDefinitionExternal{
						SourceType: &vega.DataSourceDefinitionExternal_Oracle{
							Oracle: &vega.DataSourceSpecConfiguration{
								Filters: []*oraclesv1.Filter{
									{
										Key: &oraclesv1.PropertyKey{
											Name: "trading.termination",
											Type: oraclesv1.PropertyKey_TYPE_BOOLEAN,
										},
										Conditions: []*oraclesv1.Condition{},
									},
								},
								Signers: []*oraclesv1.Signer{
									{
										Signer: &oraclesv1.Signer_PubKey{
											PubKey: &oraclesv1.PubKey{
												Key: m.pubKey,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			DataSourceSpecBinding: &vega.DataSourceSpecToFutureBinding{
				SettlementDataProperty:     "prices.ETH.value",
				TradingTerminationProperty: "trading.termination",
			},
		},
	}
}

// secondsFromNowInSecs : Creates a timestamp relative to the current time in seconds.
func secondsFromNowInSecs(seconds int64) int64 {
	return time.Now().Unix() + seconds
}
