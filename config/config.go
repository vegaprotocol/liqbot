// Package config contains structures used in retrieving app configuration
// from disk.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"code.vegaprotocol.io/protos/vega"
	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/types"
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

	// Location points to a Vega node gRPC endpoint (host:port).
	Location string `yaml:"location"`

	// ConnectTimeout is the timeout (in milliseconds) for connecting to the Vega node gRPC endpoint.
	ConnectTimeout int `yaml:"connectTimeout"`

	// CallTimeout is the per-call timeout (in milliseconds) for communicating with the Vega node gRPC endpoint.
	CallTimeout int `yaml:"callTimeout"`

	// InstrumentBase is the base asset of the instrument.
	InstrumentBase string `yaml:"instrumentBase"`

	// InstrumentQuote is the quote asset of the instrument.
	InstrumentQuote string `yaml:"instrumentQuote"`

	// Strategy specifies which algorithm the bot is to use.
	Strategy string `yaml:"strategy"`

	// SettlementAsset is the asset used for settlement.
	SettlementAsset string `yaml:"settlementAsset"`

	// StrategyDetails contains the parameters needed by the strategy algorithm.
	StrategyDetails Strategy `yaml:"strategyDetails"`
}

const (
	BotStrategyNormal = "normal"
)

// Strategy describes parameters for the bot's strategy.
type Strategy struct {
	// ExpectedMarkPrice (optional) specifies the expected mark price for a market that may not yet
	// have a mark price. It is used to calculate margin cost of orders meeting liquidity
	// requirement.
	ExpectedMarkPrice Uint `yaml:"expectedMarkPrice"`

	// AuctionVolume ...
	AuctionVolume Uint `yaml:"auctionVolume"`

	// CommitmentFraction is the fractional amount of stake for the LP
	CommitmentFraction float64 `yaml:"commitmentFraction"`

	// CommitmentAmount is the amount of stake for the LP
	CommitmentAmount string `yaml:"commitmentAmount"`

	// Fee is the 0->1 fee for supplying liquidity
	Fee string `yaml:"fee"`

	// MaxLong specifies the maximum long position that the bot will tolerate.
	MaxLong Uint `yaml:"maxLong"`

	// MaxShort specifies the maximum short position that the bot will tolerate.
	MaxShort Uint `yaml:"maxShort"`

	// PosManagementFraction controls the size of market orders used to manage the bot's position.
	PosManagementFraction float64 `yaml:"posManagementFraction"`

	// StakeFraction (along with OrdersFraction) is used in rule-of-thumb heuristics to decide how
	// the bot should deploy collateral.
	StakeFraction float64 `yaml:"stakeFraction"`

	// OrdersFraction (along with StakeFraction) is used in rule-of-thumb heuristics to decide how
	// the bot should deploy collateral.
	OrdersFraction float64 `yaml:"ordersFraction"`

	// ShorteningShape (which includes both sides of the book) specifies the shape used when the bot
	// is trying to shorten its position.
	ShorteningShape Shape `yaml:"shorteningShape"`

	// LongeningShape (which includes both sides of the book) specifies the shape used when the bot
	// is trying to lengthen its position. Note that the initial shape used by the bot is always the
	// longening shape, because being long is a little cheaper in position margin than being short.
	LongeningShape Shape `yaml:"longeningShape"`

	// PosManagementSleepMilliseconds is the sleep time, in milliseconds, between position management
	PosManagementSleepMilliseconds int `yaml:"posManagementSleepMilliseconds"`

	// MarketPriceSteeringRatePerSecond ...
	MarketPriceSteeringRatePerSecond float64 `yaml:"marketPriceSteeringRatePerSecond"`

	// MinPriceSteerFraction is the minimum difference between external and current price that will
	// allow a price steering order to be placed.
	MinPriceSteerFraction float64 `yaml:"minPriceSteerFraction"`

	// PriceSteerOrderScale is the scaling factor used when placing a steering order
	PriceSteerOrderScale float64 `yaml:"priceSteerOrderScale"`

	// LimitOrderDistributionParams ...
	LimitOrderDistributionParams LimitOrderDistParams `yaml:"limitOrderDistributionParams"`

	// TargetLNVol specifies the target log-normal volatility (e.g. 0.5 for 50%).
	TargetLNVol float64 `yaml:"targetLNVol"`
}

func (s Strategy) String() string {
	return fmt.Sprintf(
		`normal.Strategy{ExpectedMarkPrice=%d, 
AuctionVolume=%d, 
MaxLong=%d, 
MaxShort=%d, 
PosManagementFraction=%f, 
StakeFraction=%f, 
OrdersFraction=%f, 
ShorteningShape=TBD(*ShapeConfig), 
LongeningShape=TBD(*ShapeConfig), 
PosManagementSleepMilliseconds=%d, 
MarketPriceSteeringRatePerSecond=%f, 
MinPriceSteerFraction=%f, 
PriceSteerOrderScale=%f, 
LimitOrderDistributionParams=TBD(*LODParamsConfig), 
TargetLNVol=%f}`,
		s.ExpectedMarkPrice,
		s.AuctionVolume,
		s.MaxLong,
		s.MaxShort,
		s.PosManagementFraction,
		s.StakeFraction,
		s.OrdersFraction,
		// s.ShorteningShape,
		// s.LongeningShape,
		s.PosManagementSleepMilliseconds,
		s.MarketPriceSteeringRatePerSecond,
		s.MinPriceSteerFraction,
		s.PriceSteerOrderScale,
		// s.LimitOrderDistributionParams,
		s.TargetLNVol,
	)
}

func (s Strategy) validateStrategyConfig() error {
	var errs *multierror.Error

	if s.PriceSteerOrderScale <= 0 {
		errs = multierror.Append(errs, fmt.Errorf("invalid strategy config: PriceSteerOrderScale must be >0"))
	}

	if s.PosManagementSleepMilliseconds < 100 {
		errs = multierror.Append(errs, fmt.Errorf("invalid strategy config: PosManagementSleepMilliseconds must be >=100"))
	}

	if s.MarketPriceSteeringRatePerSecond <= 0.0 || s.MarketPriceSteeringRatePerSecond > 10.0 {
		errs = multierror.Append(errs, fmt.Errorf("invalid strategy config: MarketPriceSteeringRatePerSecond must be >0 and <=10"))
	}

	return errs.ErrorOrNil()
}

// LimitOrderDistParams for configuring the way price steering orders are sent.
type LimitOrderDistParams struct {
	Method              SteeringMethod `yaml:"method"`
	GttLength           uint64         `yaml:"gttLengthSeconds"`
	TgtTimeHorizonHours float64        `yaml:"tgtTimeHorizonHours"`
	NumTicksFromMid     uint64         `yaml:"numTicksFromMid"`
	NumIdenticalBots    int            `yaml:"numIdenticalBots"`
}

// Shape describes the buy and sell sides of a Liquidity Provision instruction.
type Shape struct {
	Sells LiquidityOrders `yaml:"sells"`
	Buys  LiquidityOrders `yaml:"buys"`
}

func (s Shape) ToVegaShape() types.Shape {
	return types.Shape{
		Sells: s.Sells.ToVegaLiquidityOrders(),
		Buys:  s.Buys.ToVegaLiquidityOrders(),
	}
}

type LiquidityOrders []LiquidityOrder

func (l LiquidityOrders) ToVegaLiquidityOrders() []*vega.LiquidityOrder {
	vl := make([]*vega.LiquidityOrder, len(l))

	for i := range l {
		vl[i] = l[i].ToVegaLiquidityOrder()
	}

	return vl
}

// LiquidityOrder describes ...
type LiquidityOrder struct {
	Reference  string `yaml:"reference"`
	Proportion uint32 `yaml:"proportion"`
	Offset     string `yaml:"offset"`
}

func (l LiquidityOrder) ToVegaLiquidityOrder() *vega.LiquidityOrder {
	return &vega.LiquidityOrder{
		Reference:  refStringToEnum(l.Reference),
		Proportion: l.Proportion,
		Offset:     l.Offset,
	}
}

func refStringToEnum(reference string) vega.PeggedReference {
	reference = strings.ToUpper(reference)
	switch reference {
	case "ASK":
		return vega.PeggedReference_PEGGED_REFERENCE_BEST_ASK
	case "BID":
		return vega.PeggedReference_PEGGED_REFERENCE_BEST_BID
	case "MID":
		return vega.PeggedReference_PEGGED_REFERENCE_MID
	default:
		return vega.PeggedReference_PEGGED_REFERENCE_UNSPECIFIED
	}
}

// WalletConfig describes the settings for running an internal wallet server.
type WalletConfig struct {
	URL string `yaml:"url"`
}

type SeedConfig struct {
	EthereumAddress         string `yaml:"ethereumAddress"`
	Erc20BridgeAddress      string `yaml:"erc20BridgeAddress"`
	StakingBridgeAddress    string `yaml:"stakingBridgeAddress"`
	TUSDCTokenAddress       string `yaml:"tUSDCTokenAddress"`
	VegaTokenAddress        string `yaml:"vegaTokenAddress"`
	ContractOwnerAddress    string `yaml:"contractOwnerAddress"`
	ContractOwnerPrivateKey string `yaml:"contractOwnerPrivateKey"`
	Amount                  int64  `yaml:"amount"`
}

// Config describes the top level config file format.
type Config struct {
	Server *ServerConfig `yaml:"server"`

	Pricing *PricingConfig `yaml:"pricing"`
	Wallet  *WalletConfig  `yaml:"wallet"`
	Seed    *SeedConfig    `yaml:"seed"`

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
