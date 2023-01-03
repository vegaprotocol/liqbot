// Package config contains structures used in retrieving app configuration
// from disk.
package config

import (
	"fmt"
	"net/url"

	"go.uber.org/zap"

	tconfig "code.vegaprotocol.io/shared/libs/erc20/config"
	"code.vegaprotocol.io/shared/libs/errors"
	"code.vegaprotocol.io/shared/libs/wallet"
	wconfig "code.vegaprotocol.io/shared/libs/whale/config"
	"code.vegaprotocol.io/vega/logging"
)

// Config describes the top level config file format.
type Config struct {
	Server *ServerConfig `yaml:"server"`

	CallTimeoutMills int                  `yaml:"callTimeoutMills"`
	VegaAssetID      string               `yaml:"vegaAssetID"`
	Pricing          *PricingConfig       `yaml:"pricing"`
	Wallet           *wallet.Config       `yaml:"wallet"`
	Whale            *wconfig.WhaleConfig `yaml:"whale"`
	Token            *tconfig.TokenConfig `yaml:"token"`
	Locations        []string             `yaml:"locations"`

	Bots []BotConfig `yaml:"bots"`
}

// CheckConfig checks the config for valid structure and values.
func (cfg *Config) CheckConfig() error {
	if cfg.Server == nil {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "server")
	}

	if cfg.Pricing == nil {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "pricing")
	}

	if cfg.Wallet == nil {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "wallet")
	}

	if cfg.Whale == nil {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "whale")
	}

	if cfg.Token == nil {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "token")
	}

	if len(cfg.Locations) == 0 {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "locations")
	}

	if cfg.Bots == nil || len(cfg.Bots) == 0 {
		return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "bots")
	}

	for _, bot := range cfg.Bots {
		if err := bot.StrategyDetails.validateStrategyConfig(); err != nil {
			return fmt.Errorf("failed to validate strategy config for bot '%s': %s", bot.Name, err)
		}
	}

	return nil
}

// ConfigureLogging configures logging.
func (cfg *Config) ConfigureLogging(log *logging.Logger) *logging.Logger {
	logCfg := logging.NewDefaultConfig()

	if cfg.Server.LogEncoding != "" {
		logCfg.Custom.Zap.Encoding = cfg.Server.LogEncoding
	}

	if cfg.Server.Env != "" {
		logCfg.Environment = cfg.Server.Env
	}

	log = logging.NewLoggerFromConfig(logCfg)
	if logCfg.Environment != "prod" {
		log.Logger = log.Logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	}

	if loglevel, err := logging.ParseLevel(cfg.Server.LogLevel); err == nil {
		log.SetLevel(loglevel)
	} else {
		log.SetLevel(logging.WarnLevel)
	}

	return log
}

// ServerConfig describes the settings for running the liquidity bot.
type ServerConfig struct {
	Env         string
	Listen      string
	LogEncoding string
	LogLevel    string
}

// PricingConfig describes the settings for contacting the price proxy.
type PricingConfig struct {
	Address *url.URL `yaml:"address"`
}

// BotConfig specifies the configuration parameters for one bot, which talks to one market on one
// Vega node.
type BotConfig struct {
	// Name is the name of the bot. It is also used as the wallet name.
	// It is *not* a public key seen by Vega.
	Name string `yaml:"name"`

	// InstrumentBase is the base asset of the instrument.
	InstrumentBase string `yaml:"instrumentBase"`

	// InstrumentQuote is the quote asset of the instrument.
	InstrumentQuote string `yaml:"instrumentQuote"`

	// Strategy specifies which algorithm the bot is to use.
	Strategy string `yaml:"strategy"`

	// SettlementAssetID is the asset used for settlement.
	SettlementAssetID string `yaml:"settlementAssetID"`

	// StrategyDetails contains the parameters needed by the strategy algorithm.
	StrategyDetails Strategy `yaml:"strategyDetails"`
}
