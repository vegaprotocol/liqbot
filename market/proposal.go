package market

import (
	"fmt"
	"time"

	"code.vegaprotocol.io/liqbot/util"
	"code.vegaprotocol.io/vega/protos/vega"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	oraclespb "code.vegaprotocol.io/vega/protos/vega/oracles/v1"
)

type ShortMarketProposalConfig struct {
	Name            string
	Title           string
	Description     string
	InstrumentBase  string
	InstrumentQuote string
	InstrumentCode  string

	DataSubmitterPubKey   string
	SettlementVegaAssetId string
	DecimalPlaces         uint64

	ClosingTime   time.Time
	EnactmentTime time.Time
	ExtraMetadata []string
}

func NewMarketProposal(config ShortMarketProposalConfig) *commandspb.ProposalSubmission {
	var (
		reference                     = util.RandAlpaNumericString(40)
		settlementDataPropertyKey     = fmt.Sprintf("prices.%s.value", config.InstrumentBase)
		tradingTerminationPropertyKey = fmt.Sprintf("termination.%s.value", config.InstrumentBase)
	)

	return &commandspb.ProposalSubmission{
		Reference: reference,
		Rationale: &vega.ProposalRationale{
			Title:       config.Title,
			Description: config.Description,
		},
		Terms: &vega.ProposalTerms{
			ClosingTimestamp:   config.ClosingTime.Unix(),
			EnactmentTimestamp: config.EnactmentTime.Unix(),
			Change: &vega.ProposalTerms_NewMarket{
				NewMarket: &vega.NewMarket{
					Changes: &vega.NewMarketConfiguration{
						DecimalPlaces: config.DecimalPlaces,
						Instrument: &vega.InstrumentConfiguration{
							Name: config.Name,
							Code: config.InstrumentCode,
							Product: &vega.InstrumentConfiguration_Future{
								Future: &vega.FutureProduct{
									SettlementAsset: config.SettlementVegaAssetId,
									QuoteName:       config.InstrumentQuote,
									OracleSpecForSettlementData: &oraclespb.OracleSpecConfiguration{
										PubKeys: []string{config.DataSubmitterPubKey},
										Filters: []*oraclespb.Filter{
											{
												Key: &oraclespb.PropertyKey{
													Name: settlementDataPropertyKey,
													Type: oraclespb.PropertyKey_TYPE_INTEGER,
												},
												Conditions: []*oraclespb.Condition{
													{
														Operator: oraclespb.Condition_OPERATOR_EQUALS,
														Value:    "1",
													},
												},
											},
										},
									},
									OracleSpecForTradingTermination: &oraclespb.OracleSpecConfiguration{
										PubKeys: []string{config.DataSubmitterPubKey},
										Filters: []*oraclespb.Filter{
											{
												Key: &oraclespb.PropertyKey{
													Name: tradingTerminationPropertyKey,
													Type: oraclespb.PropertyKey_TYPE_BOOLEAN,
												},
												Conditions: []*oraclespb.Condition{
													{
														Operator: oraclespb.Condition_OPERATOR_EQUALS,
														Value:    "1",
													},
												},
											},
										},
									},
									OracleSpecBinding: &vega.OracleSpecToFutureBinding{
										SettlementDataProperty:     settlementDataPropertyKey,
										TradingTerminationProperty: tradingTerminationPropertyKey,
									},
								},
							},
						},
						Metadata: append([]string{
							fmt.Sprintf("quote:%s", config.InstrumentQuote),
							fmt.Sprintf("ticker:%s", config.InstrumentBase),
							fmt.Sprintf("base:%s", config.InstrumentBase),
						}, config.ExtraMetadata...),
						PriceMonitoringParameters: &vega.PriceMonitoringParameters{
							Triggers: []*vega.PriceMonitoringTrigger{
								{
									Horizon:          43200,
									Probability:      "0.9999999",
									AuctionExtension: 600,
								},
								// {
								// 	Horizon:          300,
								// 	Probability:      "0.9999",
								// 	AuctionExtension: 60,
								// },
							},
						},
						LiquidityMonitoringParameters: &vega.LiquidityMonitoringParameters{
							TargetStakeParameters: &vega.TargetStakeParameters{
								TimeWindow:    3600,
								ScalingFactor: 10,
							},
							TriggeringRatio:  0.7, // 0.0
							AuctionExtension: 1,
						},
						RiskParameters: &vega.NewMarketConfiguration_LogNormal{
							LogNormal: &vega.LogNormalRiskModel{
								RiskAversionParameter: 0.1,             // 0.0001 // 0.01
								Tau:                   0.0001140771161, // 0.0000190129
								Params: &vega.LogNormalModelParams{
									Mu:    0,
									R:     0.016,
									Sigma: 0.3, // 0.8 // 0.5 // 1.25
								},
							},
						},
					},
				},
			},
		},
	}
}

//
// Example of predefined markets
//

func NewAAPLMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("Apple Monthly (%s)", time.Now().AddDate(0, 1, 0).Format("Jan 2006")), // Now + 1 months
			Title:                 "New USD market",
			Description:           "New USD market",
			InstrumentBase:        "AAPL",
			InstrumentQuote:       "USD",
			InstrumentCode:        "AAPL.MF21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:equities/single-stock-futures",
				"sector:tech",
				"listing_venue:NASDAQ",
				"country:US",
			}, extraMetadata...),
		},
	)
}

func NewAAVEDAIMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("AAVEDAI Monthly (%s)", time.Now().AddDate(0, 1, 0).Format("Jan 2006")), // Now + 1 months
			Title:                 "New DAI market",
			Description:           "New DAI market",
			InstrumentBase:        "AAVE",
			InstrumentQuote:       "DAI",
			InstrumentCode:        "AAVEDAI.MF21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:fx/crypto",
				"monthly",
				"sector:defi",
			}, extraMetadata...),
		},
	)
}

func NewBTCUSDMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("BTCUSD Monthly (%s)", time.Now().AddDate(0, 1, 0).Format("Jan 2006")), // Now + 1 months
			Title:                 "New BTCUSD market",
			Description:           "New BTCUSD market",
			InstrumentBase:        "BTC",
			InstrumentQuote:       "USD",
			InstrumentCode:        "BTCUSD.MF21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:fx/crypto",
				"monthly",
				"sector:crypto",
			}, extraMetadata...),
		},
	)
}

func NewETHBTCMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("ETHBTC Quarterly (%s)", time.Now().AddDate(0, 3, 0).Format("Jan 2006")), // Now + 3 months
			Title:                 "New BTC market",
			Description:           "New BTC market",
			InstrumentBase:        "ETH",
			InstrumentQuote:       "BTC",
			InstrumentCode:        "ETHBTC.QM21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:fx/crypto",
				"quarterly",
				"sector:crypto",
			}, extraMetadata...),
		},
	)
}

func NewTSLAMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("Tesla Quarterly (%s)", time.Now().AddDate(0, 3, 0).Format("Jan 2006")), // Now + 3 months
			Title:                 "New EURO market",
			Description:           "New EURO market",
			InstrumentBase:        "TSLA",
			InstrumentQuote:       "EURO",
			InstrumentCode:        "TSLA.QM21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:equities/single-stock-futures",
				"sector:tech",
				"listing_venue:NASDAQ",
				"country:US",
			}, extraMetadata...),
		},
	)
}

func NewUNIDAIMarketProposal(
	dataSubmitterPubKey string,
	settlementVegaAssetId string,
	decimalPlaces uint64,
	closingTime time.Time,
	enactmentTime time.Time,
	extraMetadata []string,
) *commandspb.ProposalSubmission {
	return NewMarketProposal(
		ShortMarketProposalConfig{
			Name:                  fmt.Sprintf("UNIDAI Monthly (%s)", time.Now().AddDate(0, 1, 0).Format("Jan 2006")), // Now + 1 month
			Title:                 "New DAI market",
			Description:           "New DAI market",
			InstrumentBase:        "UNI",
			InstrumentQuote:       "DAI",
			InstrumentCode:        "UNIDAI.MF21",
			DataSubmitterPubKey:   dataSubmitterPubKey,
			SettlementVegaAssetId: settlementVegaAssetId,
			DecimalPlaces:         decimalPlaces,
			ClosingTime:           closingTime,
			EnactmentTime:         enactmentTime,
			ExtraMetadata: append([]string{
				"class:fx/crypto",
				"monthly",
				"sector:defi",
			}, extraMetadata...),
		},
	)
}
