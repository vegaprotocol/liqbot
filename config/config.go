// Package config contains structures used in retrieving app configuration
// from disk.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
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

	// Location points to a Vega node gRPC endpoint (host:port)
	Location string `yaml:"location"`

	// ConnectTimeout is the timeout (in milliseconds) for connecting to the Vega node gRPC endpoint.
	ConnectTimeout int `yaml:"connectTimeout"`

	// CallTimeout is the per-call timeout (in milliseconds) for communicating with the Vega node gRPC endpoint.
	CallTimeout int `yaml:"callTimeout"`

	// MarketID is the Vega Market ID.
	MarketID string `yaml:"marketID"`

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

// ReadFloat64 extracts a float64 from a strategy config map.
func ReadFloat64(details map[string]string, key string) (v float64, err error) {
	value, found := details[key]
	if !found {
		err = errors.New("missing config")
		return
	}
	return strconv.ParseFloat(value, 64)
}

// ReadUint64 extracts a uint64 from a strategy config map.
func ReadUint64(details map[string]string, key string) (v uint64, err error) {
	value, found := details[key]
	if !found {
		err = errors.New("missing config")
		return
	}
	return strconv.ParseUint(value, 0, 64)
}
