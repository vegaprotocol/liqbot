package config_test

import (
	"strings"
	"testing"

	"code.vegaprotocol.io/liqbot/config"

	"github.com/stretchr/testify/assert"
)

func TestCheckConfig(t *testing.T) {
	err := config.CheckConfig(nil)
	assert.Equal(t, config.ErrNil, err)

	var cfg config.Config
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Server = &config.ServerConfig{}
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Pricing = &config.PricingConfig{}
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Wallet = &config.WalletConfig{}
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Seed = &config.SeedConfig{}
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Bots = []config.BotConfig{}
	err = config.CheckConfig(&cfg)
	assert.True(t, strings.HasPrefix(err.Error(), config.ErrMissingEmptyConfigSection.Error()))

	cfg.Bots = append(cfg.Bots, config.BotConfig{})
	err = config.CheckConfig(&cfg)
	assert.NoError(t, err)
}

func TestConfigureLogging(t *testing.T) {
	err := config.ConfigureLogging(nil)
	assert.Equal(t, config.ErrNil, err)

	var servercfg config.ServerConfig
	err = config.ConfigureLogging(&servercfg)
	assert.NoError(t, err)

	servercfg.LogLevel = "info"
	for _, lf := range []string{"json", "textcolour", "textnocolour", "fred"} {
		servercfg.LogFormat = lf
		err = config.ConfigureLogging(&servercfg)
		assert.NoError(t, err)
	}
}
