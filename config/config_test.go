package config_test

import (
	"strings"
	"testing"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/errors"

	"github.com/stretchr/testify/assert"
)

func TestCheckConfig(t *testing.T) {
	cfg := new(config.Config)

	err := cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Server = &config.ServerConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Pricing = &config.PricingConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Wallet = &config.WalletConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Whale = &config.WhaleConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Token = &config.TokenConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Locations = []string{""}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Bots = []config.BotConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	botConfig := config.BotConfig{
		Name: "test",
		StrategyDetails: config.Strategy{
			PosManagementSleepMilliseconds:   101,
			MarketPriceSteeringRatePerSecond: 1,
			PriceSteerOrderScale:             12,
		},
		InstrumentBase:    "AAPL",
		InstrumentQuote:   "USD",
		SettlementAssetID: "c9fe6fc24fce121b2cc72680543a886055abb560043fda394ba5376203b7527d",
		MarketProposalConfig: config.MarketProposalConfig{
			DataSubmitterPubKey: "0xDEADBEEF",
			InstrumentCode:      "AAPL.MF21",
			Name:                "Apple Monthly (Feb 2024)",
			Title:               "New USD market",
			Description:         "New USD market",
			DecimalPlaces:       18,
			Metadata: []string{
				"class:equities/single-stock-futures",
				"sector:tech",
				"listing_venue:NASDAQ",
				"country:US",
			},
		},
	}
	cfg.Bots = append(cfg.Bots, botConfig)
	err = cfg.CheckConfig()
	assert.NoError(t, err)
}

func TestConfigureLogging(t *testing.T) {
	cfg := new(config.Config)
	cfg.Server = &config.ServerConfig{}

	var servercfg config.ServerConfig
	err := cfg.ConfigureLogging()
	assert.NoError(t, err)

	servercfg.LogLevel = "info"
	for _, lf := range []string{"json", "textcolour", "textnocolour", "fred"} {
		servercfg.LogFormat = lf
		err = cfg.ConfigureLogging()
		assert.NoError(t, err)
	}
}
