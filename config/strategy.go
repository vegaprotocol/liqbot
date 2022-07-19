package config

import (
	"fmt"
	"strings"

	"code.vegaprotocol.io/protos/vega"
	"github.com/hashicorp/go-multierror"

	"code.vegaprotocol.io/liqbot/types"
)

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
