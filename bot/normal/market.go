package normal

import (
	"context"
	"fmt"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	v1 "code.vegaprotocol.io/protos/vega/commands/v1"
	oraclesv1 "code.vegaprotocol.io/protos/vega/oracles/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/seed"
	"code.vegaprotocol.io/liqbot/types/num"
)

func (b *Bot) createMarket(ctx context.Context) (*dataapipb.MarketsResponse, error) {
	if err := b.subscribeToStakeLinkingEvents(); err != nil {
		return nil, fmt.Errorf("failed to subscribe to stake linking events: %w", err)
	}

	b.log.Debug("minting and staking tokens")

	seedSvc, err := seed.NewService(b.seedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create seed service: %w", err)
	}

	if err := seedSvc.SeedStakeDeposit(ctx, b.walletPubKey); err != nil {
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

	if err := b.sendNewMarketProposal(ctx); err != nil {
		return nil, fmt.Errorf("failed to send new market proposal: %w", err)
	}

	b.log.Debug("waiting for proposal ID...")

	proposalID, err := b.waitForProposalID()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for proposal ID: %w", err)
	}

	b.log.Debug("successfully sent new market proposal")
	b.log.Debug("sending votes for market proposal")

	if err = b.sendVote(ctx, b.walletPubKey, proposalID, true); err != nil {
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

func (b *Bot) sendNewMarketProposal(ctx context.Context) error {
	cmd := &walletpb.SubmitTransactionRequest_ProposalSubmission{
		ProposalSubmission: b.getExampleMarketProposal(),
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:    b.walletPubKey,
		Propagate: true,
		Command:   cmd,
	}

	if err := b.signSubmitTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	return nil
}

func (b *Bot) waitForStakeLinking() error {
	select {
	case <-b.stakeLinkingCh:
		return nil
	case <-time.NewTimer(time.Second * 40).C:
		return fmt.Errorf("timeout waiting for stake linking")
	}
}

func (b *Bot) waitForProposalID() (string, error) {
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

func (b *Bot) waitForProposalEnacted(pID string) error {
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

	return nil
}

func (b *Bot) sendVote(ctx context.Context, pubKeyHex string, proposalId string, vote bool) error {
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
		PubKey:  pubKeyHex,
		Command: cmd,
	}

	if err := b.signSubmitTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit Vote Submission: %w", err)
	}

	return nil
}

func (b *Bot) seedOrders(ctx context.Context) error {
	// GTC SELL 400@1000
	if err := b.createSeedOrder(ctx, num.NewUint(1000), 400, vega.Side_SIDE_SELL, vega.Order_TIME_IN_FORCE_GTC); err != nil {
		return fmt.Errorf("failed to create seed order: %w", err)
	}

	time.Sleep(time.Second * 2)

	// GTC BUY 400@250
	if err := b.createSeedOrder(ctx, num.NewUint(250), 400, vega.Side_SIDE_BUY, vega.Order_TIME_IN_FORCE_GTC); err != nil {
		return fmt.Errorf("failed to create seed order: %w", err)
	}

	time.Sleep(time.Second * 2)

	for i := 0; !b.canPlaceOrders(); i++ {
		side := vega.Side_SIDE_BUY
		if i%2 == 0 {
			side = vega.Side_SIDE_SELL
		}

		if err := b.createSeedOrder(ctx, num.NewUint(500), 400, side, vega.Order_TIME_IN_FORCE_GFA); err != nil {
			return fmt.Errorf("failed to create seed order: %w", err)
		}

		time.Sleep(time.Second * 2)
	}

	b.log.Debug("seed orders created")
	return nil
}

func (b *Bot) createSeedOrder(ctx context.Context, price *num.Uint, size uint64, side vega.Side, tif vega.Order_TimeInForce) error {
	b.log.WithFields(log.Fields{
		"size":  size,
		"side":  side,
		"price": price,
		"tif":   tif.String(),
	}).Debug("Submitting seed order")

	if err := b.submitOrder(
		ctx,
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

func (b *Bot) getExampleMarketProposal() *v1.ProposalSubmission {
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
				NewMarket: &vega.NewMarket{
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
						Fee:              fmt.Sprint(b.strategy.Fee),
						CommitmentAmount: b.strategy.CommitmentAmount,
						Buys:             b.strategy.ShorteningShape.Buys,
						Sells:            b.strategy.LongeningShape.Sells,
					},
				},
			},
		},
	}
}

func (b *Bot) getExampleProduct() *vega.InstrumentConfiguration_Future {
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
