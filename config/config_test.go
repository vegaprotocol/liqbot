package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"code.vegaprotocol.io/liqbot/config"
	tconfig "code.vegaprotocol.io/shared/libs/erc20/config"
	"code.vegaprotocol.io/shared/libs/errors"
	"code.vegaprotocol.io/shared/libs/wallet"
	wconfig "code.vegaprotocol.io/shared/libs/whale/config"
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

	cfg.Wallet = &wallet.Config{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Whale = &wconfig.WhaleConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Token = &tconfig.TokenConfig{}
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
