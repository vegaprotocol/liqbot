package normal

import (
	"fmt"
	"time"

	"code.vegaprotocol.io/protos/vega"
	v1 "code.vegaprotocol.io/protos/vega/commands/v1"
	oraclesv1 "code.vegaprotocol.io/protos/vega/oracles/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
)

func (b *Bot) sendNewMarketProposal() error {
	cmd := &walletpb.SubmitTransactionRequest_ProposalSubmission{
		ProposalSubmission: getExampleMarketProposal(),
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:    b.walletPubKey,
		Propagate: true,
		Command:   cmd,
	}

	if err := b.signSubmitTx(submitTxReq, nil); err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	return nil
}

func (b *Bot) sendVote(pubKeyHex string, proposalId string, vote bool) error {
	value := vega.Vote_VALUE_NO
	if vote == true {
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

	if err := b.signSubmitTx(submitTxReq, nil); err != nil {
		return fmt.Errorf("failed to submit Vote Submission: %w", err)
	}

	return nil
}

func getExampleMarketProposal() *v1.ProposalSubmission {
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
							Code: "CRYPTO:BTCUSD/NOV22",
							Name: "NOV 2022 BTC vs USD future",
							Product: &vega.InstrumentConfiguration_Future{
								Future: &vega.FutureProduct{
									SettlementAsset: "993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede",
									QuoteName:       "BTCUSD",

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
							},
						},
						DecimalPlaces: 5,
						Metadata:      []string{"base:BTC", "quote:USD", "class:fx/crypto", "monthly", "sector:crypto"},
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
						Fee:              "0.01",
						CommitmentAmount: "500000000",
						Buys: []*vega.LiquidityOrder{
							{Reference: vega.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: "1600", Proportion: 25},
						},
						Sells: []*vega.LiquidityOrder{
							{Reference: vega.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: "1600", Proportion: 25},
						},
					},
				},
			},
		},
	}
}

// secondsFromNowInSecs : Creates a timestamp relative to the current time in seconds
func secondsFromNowInSecs(seconds int64) int64 {
	return time.Now().Unix() + seconds
}
