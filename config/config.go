// Package config contains structures used in retrieving app configuration
// from disk.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

// ServerConfig describes the settings for running the liquidity bot.
type ServerConfig struct {
	Env       string
	Listen    string
	LogFormat string
	LogLevel  string
}

// NodeConfig describes the settings for contacting Vega nodes.
type NodeConfig struct {
	Name string `yaml:"name"`

	// Address specifies the URL of the Vega node to connect to.
	// e.g. REST node:
	//      address:
	//        scheme: https
	//        host: node.example.com:8443
	//        path: /
	// e.g. gRPC node:
	//      address:
	//        scheme: grpc
	//        host: node.example.com:1234
	Address *url.URL `yaml:"address"`
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

	// Location is the name of a Node defined above
	Location string `yaml:"location"`

	// Strategy specifies which algorithm the bot is to use.
	Strategy string `yaml:"strategy"`

	// StrategyDetails contains the parameters needed by the strategy algorithm
	StrategyDetails map[string]string `yaml:"strategyDetails"`
}

// WalletConfig describes the settings for running an internal wallet server
type WalletConfig struct {
	RootPath    string `yaml:"rootPath"`
	TokenExpiry int    `yaml:"tokenExpiry"`
}

// Config describes the top level config file format.
type Config struct {
	Server *ServerConfig `yaml:"server"`

	Nodes   []NodeConfig   `yaml:"nodes"`
	Pricing *PricingConfig `yaml:"pricing"`
	Wallet  *WalletConfig  `yaml:"wallet"`

	Bots []BotConfig `yaml:"bots"`
}

var (
	// ErrNil indicates that a nil/null pointer was encountered
	ErrNil = errors.New("nil pointer")

	// ErrMissingEmptyConfigSection indicates that a required config file section is missing (not present) or empty (zero-length).
	ErrMissingEmptyConfigSection = errors.New("config file section is missing/empty")

	// ErrInvalidValue indicates that a value was invalid.
	ErrInvalidValue = errors.New("invalid value")
)

// CheckConfig checks the config for valid structure and values.
func CheckConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNil
	}

	if cfg.Server == nil {
		return fmt.Errorf("%s: %s", ErrMissingEmptyConfigSection.Error(), "server")
	}
	if cfg.Nodes == nil || len(cfg.Nodes) == 0 {
		return fmt.Errorf("%s: %s", ErrMissingEmptyConfigSection.Error(), "nodes")
	}
	if cfg.Pricing == nil {
		return fmt.Errorf("%s: %s", ErrMissingEmptyConfigSection.Error(), "pricing")
	}
	if cfg.Wallet == nil {
		return fmt.Errorf("%s: %s", ErrMissingEmptyConfigSection.Error(), "wallet")
	}
	if cfg.Bots == nil || len(cfg.Bots) == 0 {
		return fmt.Errorf("%s: %s", ErrMissingEmptyConfigSection.Error(), "bots")
	}

	return nil
}

// ConfigureLogging configures logging.
func ConfigureLogging(cfg *ServerConfig) error {
	if cfg == nil {
		return ErrNil
	}

	if cfg.Env != "prod" {
		// https://github.com/sirupsen/logrus#logging-method-name
		// This slows down logging (by a factor of 2).
		log.SetReportCaller(true)
	}

	switch cfg.LogFormat {
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

	if loglevel, err := log.ParseLevel(cfg.LogLevel); err == nil {
		log.SetLevel(loglevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	return nil
}
