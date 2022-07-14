package config_test

import (
	"strings"
	"testing"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/errors"

	"github.com/stretchr/testify/assert"
)

func TestCheckConfig(t *testing.T) {
	var cfg *config.Config
	err := cfg.CheckConfig()
	assert.Equal(t, errors.ErrNil, err)

	cfg = new(config.Config)

	err = cfg.CheckConfig()
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

	cfg.Seed = &config.SeedConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Bots = []config.BotConfig{}
	err = cfg.CheckConfig()
	assert.True(t, strings.HasPrefix(err.Error(), errors.ErrMissingEmptyConfigSection.Error()))

	cfg.Bots = append(cfg.Bots, config.BotConfig{})
	err = cfg.CheckConfig()
	assert.NoError(t, err)
}

func TestConfigureLogging(t *testing.T) {
	var cfg *config.Config
	err := cfg.ConfigureLogging()
	assert.Equal(t, errors.ErrNil, err)

	cfg = new(config.Config)
	cfg.Server = &config.ServerConfig{}

	var servercfg config.ServerConfig
	err = cfg.ConfigureLogging()
	assert.NoError(t, err)

	servercfg.LogLevel = "info"
	for _, lf := range []string{"json", "textcolour", "textnocolour", "fred"} {
		servercfg.LogFormat = lf
		err = cfg.ConfigureLogging()
		assert.NoError(t, err)
	}
}
