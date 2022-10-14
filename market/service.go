package market

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	v12 "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	"code.vegaprotocol.io/vega/protos/vega"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	v1 "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type Service struct {
	name          string
	pricingEngine PricingEngine
	marketStream  marketStream
	node          tradingDataService
	walletClient  normal.WalletClient // TODO: wtf?!
	store         data.MarketStore
	account       accountService
	config        config.BotConfig
	networkParams *types.NetworkParameters
	log           *log.Entry

	decimalPlaces uint64
	marketID      string
	walletPubKey  string
	vegaAssetID   string
}

func NewService(
	name string,
	marketStream marketStream,
	node tradingDataService,
	walletClient normal.WalletClient,
	pe PricingEngine,
	account accountService,
	config config.BotConfig,
	vegaAssetID string,
) *Service {
	s := &Service{
		name:          name,
		marketStream:  marketStream,
		node:          node,
		walletClient:  walletClient,
		pricingEngine: pe,
		account:       account,
		config:        config,
		vegaAssetID:   vegaAssetID,
		log:           log.WithField("component", "MarketService"),
	}

	s.log = s.log.WithFields(log.Fields{"node": s.node.Target()})
	s.log.Info("Connected to Vega gRPC node")

	return s
}

func (m *Service) Init(pubKey string, pauseCh chan types.PauseSignal) error {
	store, err := m.marketStream.Init(pubKey, pauseCh)
	if err != nil {
		return err
	}
	networkParams, err := m.node.GetAllNetworkParameters()
	if err != nil {
		return err
	}
	m.store = store
	m.walletPubKey = pubKey
	m.networkParams = networkParams
	return nil
}

func (m *Service) Start(marketID string) error {
	m.log.Info("Starting market service")
	if err := m.marketStream.Subscribe(marketID); err != nil {
		return fmt.Errorf("failed to subscribe to market stream: %w", err)
	}
	m.marketID = marketID
	return nil
}

func (m *Service) SetPubKey(pubKey string) {
	m.walletPubKey = pubKey
}

func (m *Service) Market() types.MarketData {
	return m.store.Market()
}

func (m *Service) SetupMarket(ctx context.Context) (*vega.Market, error) {
	market, err := m.FindMarket()
	if err == nil {
		m.log.WithField("market", market).Info("Found market")
		return market, nil
	}

	m.log.WithError(err).Info("Failed to find market, creating it")

	if err = m.CreateMarket(ctx); err != nil {
		return nil, fmt.Errorf("failed to create market: %w", err)
	}

	market, err = m.FindMarket()
	if err != nil {
		return nil, fmt.Errorf("failed to find market after creation: %w", err)
	}

	return market, nil
}

func (m *Service) FindMarket() (*vega.Market, error) {
	marketsResponse, err := m.node.Markets(&v12.MarketsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}

	for _, mkt := range marketsResponse.Markets {
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

		m.log = m.log.WithFields(log.Fields{"marketID": mkt.Id})
		m.decimalPlaces = mkt.DecimalPlaces

		return mkt, nil
	}

	return nil, fmt.Errorf("failed to find futures markets: base/ticker=%s, quote=%s", m.config.InstrumentBase, m.config.InstrumentQuote)
}

func (m *Service) CreateMarket(ctx context.Context) error {
	m.log.Info("Minting, staking and depositing tokens")

	seedAmount := m.config.StrategyDetails.SeedAmount.Get()

	m.log.WithFields(log.Fields{
		"amount": seedAmount.String(),
		"asset":  m.config.SettlementAssetID,
		"name":   m.name,
	}).Info("Ensuring balance for market creation")

	// TODO: is it m.settlementAssetID?
	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, seedAmount, "MarketCreation"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	m.log.WithFields(log.Fields{
		"amount": seedAmount.String(),
		"asset":  m.config.SettlementAssetID,
		"name":   m.name,
	}).Info("Balance ensured")

	m.log.WithFields(log.Fields{
		"amount": seedAmount.String(),
		"asset":  m.vegaAssetID,
		"name":   m.name,
	}).Info("Ensuring stake for market creation")

	if err := m.account.EnsureStake(ctx, m.config.Name, m.walletPubKey, m.vegaAssetID, seedAmount, "MarketCreation"); err != nil {
		return fmt.Errorf("failed to ensure stake: %w", err)
	}

	m.log.WithFields(log.Fields{
		"amount": seedAmount.String(),
		"asset":  m.vegaAssetID,
		"name":   m.name,
	}).Info("Successfully linked stake")

	m.log.Info("Sending new market proposal...")

	if err := m.sendNewMarketProposal(ctx); err != nil {
		return fmt.Errorf("failed to send new market proposal: %w", err)
	}

	m.log.Debug("Waiting for proposal ID...")

	proposalID, err := m.marketStream.WaitForProposalID()
	if err != nil {
		return fmt.Errorf("failed to wait for proposal ID: %w", err)
	}

	m.log.Debug("Successfully sent new market proposal")
	m.log.Debug("Sending votes for market proposal")

	if err = m.sendVote(ctx, proposalID, true); err != nil {
		return fmt.Errorf("failed to send vote: %w", err)
	}

	m.log.Debug("Waiting for proposal to be enacted...")

	if err = m.marketStream.WaitForProposalEnacted(proposalID); err != nil {
		return fmt.Errorf("failed to wait for proposal to be enacted: %w", err)
	}

	m.log.Debug("Market proposal successfully enacted")

	return nil
}

func (m *Service) sendNewMarketProposal(ctx context.Context) error {
	proposalParams, err := m.networkParams.GetMarketProposalParams()
	if err != nil {
		return err
	}
	marketProposal := NewMarketProposal(ShortMarketProposalConfig{
		Name:                  m.config.MarketProposalConfig.Name,
		Title:                 m.config.MarketProposalConfig.Title,
		Description:           m.config.MarketProposalConfig.Description,
		InstrumentBase:        m.config.InstrumentBase,
		InstrumentQuote:       m.config.InstrumentQuote,
		InstrumentCode:        m.config.MarketProposalConfig.InstrumentCode,
		DataSubmitterPubKey:   m.config.MarketProposalConfig.DataSubmitterPubKey,
		SettlementVegaAssetId: m.config.SettlementAssetID,
		DecimalPlaces:         m.config.MarketProposalConfig.DecimalPlaces,
		ExtraMetadata:         m.config.MarketProposalConfig.Metadata,
		ClosingTime:           time.Now().Add(time.Second * 20).Add(proposalParams.MinClose),
		EnactmentTime:         time.Now().Add(time.Second * 30).Add(proposalParams.MinClose).Add(proposalParams.MinEnact),
	})
	cmd := &walletpb.SubmitTransactionRequest_ProposalSubmission{
		ProposalSubmission: marketProposal,
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  m.walletPubKey,
		Command: cmd,
	}

	if err := m.walletClient.SignTx(ctx, submitTxReq); err != nil {
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
		PubKey:  m.walletPubKey,
		Command: cmd,
	}

	if err := m.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit Vote Submission: %w", err)
	}

	return nil
}

func (m *Service) CanPlaceOrders() bool {
	return m.Market().TradingMode() == vega.Market_TRADING_MODE_CONTINUOUS
}

// TODO: make retryable.
func (m *Service) SubmitOrder(ctx context.Context, order *vega.Order, from string, secondsFromNow int64) error {
	// TODO: is it ok to ensure balance here?

	price, overflow := num.UintFromString(order.Price, 10)
	if overflow {
		return fmt.Errorf("failed to parse price: overflow")
	}

	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, price, from); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

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

	m.log.WithFields(log.Fields{
		"reference": order.Reference,
		"size":      order.Size,
		"side":      order.Side.String(),
		"price":     order.Price,
		"tif":       order.TimeInForce.String(),
	}).Debugf("%s: Submitting order", from)

	if order.TimeInForce == vega.Order_TIME_IN_FORCE_GTT {
		cmd.OrderSubmission.ExpiresAt = time.Now().UnixNano() + (secondsFromNow * 1000000000)
	}

	if order.Type != vega.Order_TYPE_MARKET {
		cmd.OrderSubmission.Price = order.Price
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  m.walletPubKey,
		Command: cmd,
	}

	if err := m.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit OrderSubmission: %w", err)
	}

	return nil
}

func (m *Service) SeedOrders(ctx context.Context, from string) error {
	m.log.Debugf("%s: Seeding orders", from)

	externalPrice, err := m.GetExternalPrice()
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	for i := 0; !m.CanPlaceOrders(); i++ {
		price := externalPrice.Clone()
		tif := vega.Order_TIME_IN_FORCE_GFA

		side := vega.Side_SIDE_BUY
		if i%2 == 0 {
			side = vega.Side_SIDE_SELL
		}

		if i == 0 {
			price = num.UintChain(price).Mul(num.NewUint(105)).Div(num.NewUint(100)).Get()
			tif = vega.Order_TIME_IN_FORCE_GTC
		} else if i == 1 {
			price = num.UintChain(price).Mul(num.NewUint(95)).Div(num.NewUint(100)).Get()
			tif = vega.Order_TIME_IN_FORCE_GTC
		}

		order := &vega.Order{
			MarketId:    m.marketID,
			Size:        m.config.StrategyDetails.SeedOrderSize,
			Price:       price.String(),
			Side:        side,
			TimeInForce: tif,
			Type:        vega.Order_TYPE_LIMIT,
			Reference:   "MarketCreation",
		}

		if err = m.SubmitOrder(ctx, order, from, int64(m.config.StrategyDetails.PosManagementFraction)); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		time.Sleep(time.Second * 2)

		if i == 100 { // TODO: make this configurable
			return fmt.Errorf("seeding orders did not end the auction")
		}
	}

	m.log.Debugf("%s: Seeding orders finished", from)
	return nil
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

	externalPrice := externalPriceResponse.Price * math.Pow(10, float64(m.decimalPlaces))
	externalPriceNum := num.NewUint(uint64(externalPrice))
	return externalPriceNum, nil
}
