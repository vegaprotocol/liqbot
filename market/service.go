package market

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	itypes "code.vegaprotocol.io/liqbot/types"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/shared/libs/types"
	v12 "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	v1 "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	oraclesv1 "code.vegaprotocol.io/vega/protos/vega/data/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type Service struct {
	name          string
	pricingEngine itypes.PricingEngine
	marketStream  marketStream
	node          dataNode
	walletClient  normal.WalletClient // TODO: wtf?!
	store         marketStore
	account       accountService
	config        config.BotConfig
	log           *log.Entry

	decimalPlaces uint64
	marketID      string
	walletPubKey  string
	vegaAssetID   string
}

func NewService(
	name string,
	node dataNode,
	walletClient normal.WalletClient,
	pe itypes.PricingEngine,
	account accountService,
	config config.BotConfig,
	vegaAssetID string,
) *Service {
	s := &Service{
		name:          name,
		marketStream:  NewMarketStream(name, node),
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
	m.store = store
	m.walletPubKey = pubKey
	return nil
}

func (m *Service) Start(ctx context.Context, marketID string) error {
	m.log.Info("Starting market service")
	if err := m.marketStream.Subscribe(ctx, marketID); err != nil {
		return fmt.Errorf("failed to subscribe to market stream: %w", err)
	}
	m.marketID = marketID
	return nil
}

func (m *Service) Market() cache.MarketData {
	return m.store.Market()
}

func (m *Service) SetupMarket(ctx context.Context) (*vega.Market, error) {
	market, err := m.FindMarket(ctx)
	if err == nil {
		m.log.WithField("market", market).Info("Found market")
		return market, nil
	}

	m.log.WithError(err).Info("Failed to find market, creating it")

	if err = m.CreateMarket(ctx); err != nil {
		return nil, fmt.Errorf("failed to create market: %w", err)
	}

	market, err = m.FindMarket(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find market after creation: %w", err)
	}

	return market, nil
}

func (m *Service) FindMarket(ctx context.Context) (*vega.Market, error) {
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

	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.General, seedAmount, 1, "MarketCreation"); err != nil {
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
	price, overflow := num.UintFromString(order.Price, 10)
	if overflow {
		return fmt.Errorf("failed to parse price: overflow")
	}

	price.Mul(price, num.NewUint(order.Size))

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

	m.log.WithFields(log.Fields{
		"expiresAt": cmd.OrderSubmission.ExpiresAt,
		"types":     order.Type.String(),
		"reference": order.Reference,
		"size":      order.Size,
		"side":      order.Side.String(),
		"price":     order.Price,
		"tif":       order.TimeInForce.String(),
	}).Debugf("%s: Submitting order", from)

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
	externalPrice, err := m.GetExternalPrice()
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	m.log.WithFields(log.Fields{"externalPrice": externalPrice.String()}).Debugf("%s: Seeding auction orders", from)
	orders, totalCost := m.createSeedOrders(externalPrice.Clone())

	if err := m.account.EnsureBalance(ctx, m.config.SettlementAssetID, cache.General, totalCost, 1, from); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	for _, order := range orders {
		if err = m.SubmitOrder(ctx, order, from, int64(m.config.StrategyDetails.PosManagementFraction)); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		if m.CanPlaceOrders() {
			m.log.Debugf("%s: Seeding orders finished", from)
			return nil
		}

		time.Sleep(time.Second * 2)
	}

	return fmt.Errorf("seeding orders did not end the auction")
}

func (m *Service) createSeedOrders(externalPrice *num.Uint) ([]*vega.Order, *num.Uint) {
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

		add := num.NewUint(uint64(math.Abs(float64(rand.Intn(100) - 50))))
		price := num.Zero().Add(externalPrice, add)

		totalCost.Add(totalCost, price.Clone().Mul(price, num.NewUint(size)))

		if i > 1 {
			tif = vega.Order_TIME_IN_FORCE_GFA
		}

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

	externalPrice := externalPriceResponse.Price * math.Pow(10, float64(m.decimalPlaces))
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
												Key: "0xDEADBEEF",
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
												Key: "0xDEADBEEF",
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
