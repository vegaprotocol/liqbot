package normal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	commandspb "code.vegaprotocol.io/protos/vega/commands/v1"
	v1 "code.vegaprotocol.io/protos/vega/commands/v1"
	oraclesv1 "code.vegaprotocol.io/protos/vega/oracles/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/seed"
	"code.vegaprotocol.io/liqbot/types/num"
)

func (b *bot) setupMarket() error {
	marketsResponse, err := b.node.Markets(&dataapipb.MarketsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get markets: %w", err)
	}

	var market *vega.Market

	if len(marketsResponse.Markets) == 0 {
		market, err = b.createMarket(context.Background())
		if err != nil {
			return fmt.Errorf("failed to create market: %w", err)
		}
		marketsResponse.Markets = append(marketsResponse.Markets, market)
	}

	market, err = b.findMarket(marketsResponse.Markets)
	if err != nil {
		return fmt.Errorf("failed to find market: %w", err)
	}

	b.marketID = market.Id
	b.decimalPlaces = int(market.DecimalPlaces)
	b.settlementAssetID = market.TradableInstrument.Instrument.GetFuture().SettlementAsset
	b.log = b.log.WithFields(log.Fields{"marketID": b.marketID})

	return nil
}

func (b *bot) findMarket(markets []*vega.Market) (*vega.Market, error) {
	for _, mkt := range markets {
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

		if base != b.config.InstrumentBase || quote != b.config.InstrumentQuote {
			continue
		}

		return mkt, nil
	}

	return nil, fmt.Errorf("failed to find futures markets: base/ticker=%s, quote=%s", b.config.InstrumentBase, b.config.InstrumentQuote)
}

func (b *bot) createMarket(ctx context.Context) (*vega.Market, error) {
	if err := b.marketStream.Subscribe(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to market stream: %w", err)
	}

	b.log.Debug("minting and staking tokens")
	seedSvc, err := seed.NewService(b.seedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create seed service: %w", err)
	}

	if err = seedSvc.SeedStakeDeposit(ctx, b.walletPubKey); err != nil {
		return nil, fmt.Errorf("failed to seed stake tokens: %w", err)
	}

	b.log.Debug("Waiting for stake to propagate...")

	if err = b.marketStream.WaitForStakeLinking(); err != nil {
		return nil, fmt.Errorf("failed stake linking: %w", err)
	}

	b.log.Debug("Successfully linked stake")
	b.log.Debug("Sending new market proposal")

	if err = b.sendNewMarketProposal(ctx); err != nil {
		return nil, fmt.Errorf("failed to send new market proposal: %w", err)
	}

	b.log.Debug("Waiting for proposal ID...")

	proposalID, err := b.marketStream.WaitForProposalID()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for proposal ID: %w", err)
	}

	b.log.Debug("Successfully sent new market proposal")
	b.log.Debug("Sending votes for market proposal")

	if err = b.sendVote(ctx, proposalID, true); err != nil {
		return nil, fmt.Errorf("failed to send vote: %w", err)
	}

	b.log.Debug("Waiting for proposal to be enacted...")

	if err = b.marketStream.WaitForProposalEnacted(proposalID); err != nil {
		return nil, fmt.Errorf("failed to wait for proposal to be enacted: %w", err)
	}

	b.log.Debug("Market proposal successfully enacted")

	marketsResponse, err := b.node.Markets(&dataapipb.MarketsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}

	if len(marketsResponse.Markets) == 0 {
		return nil, errors.New("no markets created")
	}

	return marketsResponse.Markets[0], nil
}

func (b *bot) sendNewMarketProposal(ctx context.Context) error {
	cmd := &walletpb.SubmitTransactionRequest_ProposalSubmission{
		ProposalSubmission: b.getExampleMarketProposal(),
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:    b.walletPubKey,
		Propagate: true,
		Command:   cmd,
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	return nil
}

func (b *bot) sendVote(ctx context.Context, proposalId string, vote bool) error {
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
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit Vote Submission: %w", err)
	}

	return nil
}

func (b *bot) submitOrder(
	ctx context.Context,
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
			MarketId:    b.marketID,
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

	b.log.WithFields(log.Fields{
		"reference": reference,
		"size":      size,
		"side":      side,
		"price":     price,
		"tif":       tif.String(),
	}).Debug("Submitting order")

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

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit OrderSubmission: %w", err)
	}

	return nil
}

func (b *bot) seedOrders(ctx context.Context) error {
	b.log.Debug("Seeding orders")

	externalPrice, err := b.getExternalPrice()
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	for i := 0; !b.canPlaceOrders(); i++ {
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

		if err := b.submitOrder(ctx,
			400,
			price,
			side,
			tif,
			vega.Order_TYPE_LIMIT,
			"MarketCreation",
			int64(b.config.StrategyDetails.PosManagementFraction),
		); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		time.Sleep(time.Second * 2)

		if i == 100 { // TODO: make this configurable
			return fmt.Errorf("seeding orders did not end the auction")
		}
	}

	b.log.Debug("Seeding orders finished")
	return nil
}

func (b *bot) canPlaceOrders() bool {
	return b.data.TradingMode() == vega.Market_TRADING_MODE_CONTINUOUS
}

func (b *bot) getExampleMarketProposal() *v1.ProposalSubmission {
	return &v1.ProposalSubmission{
		Rationale: &vega.ProposalRationale{
			Description: "some description",
		},
		Reference: "ProposalReference",
		Terms: &vega.ProposalTerms{
			ValidationTimestamp: secondsFromNowInSecs(1),
			ClosingTimestamp:    secondsFromNowInSecs(10),
			EnactmentTimestamp:  secondsFromNowInSecs(15),
			Change: &vega.ProposalTerms_NewMarket{
				NewMarket: b.getExampleMarket(),
			},
		},
	}
}

func (b *bot) getExampleMarket() *vega.NewMarket {
	return &vega.NewMarket{
		Changes: &vega.NewMarketConfiguration{
			Instrument: &vega.InstrumentConfiguration{
				Code:    fmt.Sprintf("CRYPTO:%s%s/NOV22", b.config.InstrumentBase, b.config.InstrumentQuote),
				Name:    fmt.Sprintf("NOV 2022 %s vs %s future", b.config.InstrumentBase, b.config.InstrumentQuote),
				Product: b.getExampleProduct(),
			},
			DecimalPlaces: 5,
			Metadata:      []string{"base:" + b.config.InstrumentBase, "quote:" + b.config.InstrumentQuote, "class:fx/crypto", "monthly", "sector:crypto"},
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
		LiquidityCommitment: &vega.NewMarketCommitment{
			Fee:              fmt.Sprint(b.config.StrategyDetails.Fee),
			CommitmentAmount: b.config.StrategyDetails.CommitmentAmount,
			Buys:             b.config.StrategyDetails.ShorteningShape.Buys.ToVegaLiquidityOrders(),
			Sells:            b.config.StrategyDetails.LongeningShape.Sells.ToVegaLiquidityOrders(),
		},
	}
}

func (b *bot) getExampleProduct() *vega.InstrumentConfiguration_Future {
	return &vega.InstrumentConfiguration_Future{
		Future: &vega.FutureProduct{
			SettlementAsset: b.config.SettlementAsset,
			QuoteName:       fmt.Sprintf("%s%s", b.config.InstrumentBase, b.config.InstrumentQuote),
			OracleSpecForSettlementPrice: &oraclesv1.OracleSpecConfiguration{
				PubKeys: []string{"0xDEADBEEF"},
				Filters: []*oraclesv1.Filter{
					{
						Key: &oraclesv1.PropertyKey{
							Name: "prices.ETH.value",
							Type: oraclesv1.PropertyKey_TYPE_INTEGER,
						},
						Conditions: []*oraclesv1.Condition{},
					},
				},
			},
			OracleSpecForTradingTermination: &oraclesv1.OracleSpecConfiguration{
				PubKeys: []string{"0xDEADBEEF"},
				Filters: []*oraclesv1.Filter{
					{
						Key: &oraclesv1.PropertyKey{
							Name: "trading.termination",
							Type: oraclesv1.PropertyKey_TYPE_BOOLEAN,
						},
						Conditions: []*oraclesv1.Condition{},
					},
				},
			},
			OracleSpecBinding: &vega.OracleSpecToFutureBinding{
				SettlementPriceProperty:    "prices.ETH.value",
				TradingTerminationProperty: "trading.termination",
			},
		},
	}
}

// secondsFromNowInSecs : Creates a timestamp relative to the current time in seconds.
func secondsFromNowInSecs(seconds int64) int64 {
	return time.Now().Unix() + seconds
}
