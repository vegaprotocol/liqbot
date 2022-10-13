// Package config contains structures used in retrieving app configuration
// from disk.
package config

import (
	"fmt"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/errors"
)

// Config describes the top level config file format.
type Config struct {
	Server *ServerConfig `yaml:"server"`

	CallTimeoutMills int            `yaml:"callTimeoutMills"`
	VegaAssetID      string         `yaml:"vegaAssetID"`
	Pricing          *PricingConfig `yaml:"pricing"`
	Wallet           *WalletConfig  `yaml:"wallet"`
	Whale            *WhaleConfig   `yaml:"whale"`
	Token            *TokenConfig   `yaml:"token"`
	Locations        []string       `yaml:"locations"`

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
		if len(bot.DataSubmitterPubKey) == 0 {
			return fmt.Errorf("%s: %s", errors.ErrMissingEmptyConfigSection.Error(), "bots.dataSubmitterPubKey")
		}
	}

	return nil
}

// ConfigureLogging configures logging.
func (cfg *Config) ConfigureLogging() error {
	if cfg.Server.Env != "prod" {
		// https://github.com/sirupsen/logrus#logging-method-name
		// This slows down logging (by a factor of 2).
		log.SetReportCaller(true)
	}

	switch cfg.Server.LogFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	case "textcolour":
		log.SetFormatter(&log.TextFormatter{
			ForceColors:     true,
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339Nano,
		})
	case "textnocolour":
		log.SetFormatter(&log.TextFormatter{
			DisableColors:   true,
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339Nano,
		})
	default:
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339Nano,
		}) // with colour if TTY, without otherwise
	}

	if loglevel, err := log.ParseLevel(cfg.Server.LogLevel); err == nil {
		log.SetLevel(loglevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	return nil
}

// ServerConfig describes the settings for running the liquidity bot.
type ServerConfig struct {
	Env       string
	Listen    string
	LogFormat string
	LogLevel  string
}

// PricingConfig describes the settings for contacting the price proxy.
type PricingConfig struct {
	Address *url.URL `yaml:"address"`
}

type WhaleConfig struct {
	WalletPubKey     string            `yaml:"walletPubKey"`
	WalletName       string            `yaml:"walletName"`
	WalletPassphrase string            `yaml:"walletPassphrase"`
	OwnerPrivateKeys map[string]string `yaml:"ownerPrivateKeys"`
	FaucetURL        string            `yaml:"faucetURL"`
	SyncTimeoutSec   int               `yaml:"syncTimeoutSec"`
	SlackConfig      SlackConfig       `yaml:"slack"`
}

type SlackConfig struct {
	AppToken  string `yaml:"appToken"`
	BotToken  string `yaml:"botToken"`
	ChannelID string `yaml:"channelID"`
	Enabled   bool   `yaml:"enabled"`
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

	// QuoteAssetID is the id of the quote asset.
	QuoteAssetID string `yaml:"quoteAssetID"`

	// Strategy specifies which algorithm the bot is to use.
	Strategy string `yaml:"strategy"`

	// SettlementAssetID is the asset used for settlement.
	SettlementAssetID string `yaml:"settlementAssetID"`

	// DataSubmitterPubKey is a vega pub key of a party that can set settlement price and terminate market.
	DataSubmitterPubKey string `yaml:"dataSubmitterPubKey"`

	// StrategyDetails contains the parameters needed by the strategy algorithm.
	StrategyDetails Strategy `yaml:"strategyDetails"`
}

// WalletConfig describes the settings for running an internal wallet server.
type WalletConfig struct {
	URL string `yaml:"url"`
}

type TokenConfig struct {
	EthereumAPIAddress   string `yaml:"ethereumAPIAddress"`
	Erc20BridgeAddress   string `yaml:"erc20BridgeAddress"`
	StakingBridgeAddress string `yaml:"stakingBridgeAddress"`
	SyncTimeoutSec       int    `yaml:"syncTimeoutSec"`
}
