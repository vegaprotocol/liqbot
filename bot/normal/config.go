package normal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"code.vegaprotocol.io/liqbot/config"

	"code.vegaprotocol.io/protos/vega"
	"github.com/hashicorp/go-multierror"
)

// ShapeConfig is the top level definition of a liquidity shape.
type ShapeConfig struct {
	Sells []*vega.LiquidityOrder
	Buys  []*vega.LiquidityOrder
}

// LODParamsConfig is a little data structure which sets the algo and params for how limits
// orders are generated.
type LODParamsConfig struct {
	Method              SteeringMethod
	GttLength           uint64
	TgtTimeHorizonHours float64
	NumTicksFromMid     uint64
	NumIdenticalBots    int
}

// Strategy configures the normal strategy.
type Strategy struct {
	// ExpectedMarkPrice (optional) specifies the expected mark price for a market that may not yet
	// have a mark price. It is used to calculate margin cost of orders meeting liquidity
	// requirement.
	ExpectedMarkPrice config.Uint

	// AuctionVolume ...
	AuctionVolume config.Uint

	// CommitmentFraction is the fractional amount of stake for the LP
	CommitmentFraction float64

	// Fee is the 0->1 fee for supplying liquidity
	Fee float64

	// MaxLong specifies the maximum long position that the bot will tolerate.
	MaxLong config.Uint

	// MaxShort specifies the maximum short position that the bot will tolerate.
	MaxShort config.Uint

	// PosManagementFraction controls the size of market orders used to manage the bot's position.
	PosManagementFraction float64

	// StakeFraction (along with OrdersFraction) is used in rule-of-thumb heuristics to decide how
	// the bot should deploy collateral.
	StakeFraction float64

	// OrdersFraction (along with StakeFraction) is used in rule-of-thumb heuristics to decide how
	// the bot should deploy collateral.
	OrdersFraction float64

	// ShorteningShape (which includes both sides of the book) specifies the shape used when the bot
	// is trying to shorten its position.
	ShorteningShape *ShapeConfig

	// LongeningShape (which includes both sides of the book) specifies the shape used when the bot
	// is trying to lengthen its position. Note that the initial shape used by the bot is always the
	// longening shape, because being long is a little cheaper in position margin than being short.
	LongeningShape *ShapeConfig

	// PosManagementSleepMilliseconds is the sleep time, in milliseconds, between position management
	PosManagementSleepMilliseconds uint64

	// MarketPriceSteeringRatePerSecond ...
	MarketPriceSteeringRatePerSecond float64

	// MinPriceSteerFraction is the minimum difference between external and current price that will
	// allow a price steering order to be placed.
	MinPriceSteerFraction float64

	// PriceSteerOrderScale is the scaling factor used when placing a steering order
	PriceSteerOrderScale float64

	// LimitOrderDistributionParams ...
	LimitOrderDistributionParams *LODParamsConfig

	// TargetLNVol specifies the target log-normal volatility (e.g. 0.5 for 50%).
	TargetLNVol float64
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

// SteeringMethod is an enum for all the possible price calculations methods for price steering.
type SteeringMethod int

const (
	// NotSet for when we cannot parse the input string.
	NotSet SteeringMethod = iota
	// DiscreteThreeLevel uses the discrete three level method.
	DiscreteThreeLevel
	// CoinAndBinomial uses the coin and binomial method.
	CoinAndBinomial
)

func steeringMethodToEnum(method string) (SteeringMethod, error) {
	switch method {
	case "discreteThreeLevel":
		return DiscreteThreeLevel, nil
	case "coinAndBinomial":
		return CoinAndBinomial, nil
	}
	return NotSet, fmt.Errorf("Steering method unknown:%s", method)
}

func validateStrategyConfig(details config.Strategy) (s *Strategy, err error) {
	s = &Strategy{}
	errInvalid := "invalid strategy config for %s: %w"

	var errs *multierror.Error

	s.ExpectedMarkPrice = details.ExpectedMarkPrice
	s.AuctionVolume = details.AuctionVolume
	s.MaxLong = details.MaxLong
	s.MaxShort = details.MaxShort
	s.PosManagementFraction = details.PosManagementFraction
	s.StakeFraction = details.StakeFraction
	s.OrdersFraction = details.OrdersFraction
	s.CommitmentFraction = details.CommitmentFraction
	s.Fee, _ = strconv.ParseFloat(details.Fee, 64)

	var shorteningShape *ShapeConfig = &ShapeConfig{
		Sells: []*vega.LiquidityOrder{},
		Buys:  []*vega.LiquidityOrder{},
	}

	var longeningShape *ShapeConfig = &ShapeConfig{
		Sells: []*vega.LiquidityOrder{},
		Buys:  []*vega.LiquidityOrder{},
	}

	for _, buy := range details.ShorteningShape.Buys {
		shorteningShape.Buys = append(shorteningShape.Buys, &vega.LiquidityOrder{
			Reference:  refStringToEnum(buy.Reference),
			Proportion: buy.Proportion,
			Offset:     buy.Offset,
		})
	}
	for _, sell := range details.ShorteningShape.Sells {
		shorteningShape.Sells = append(shorteningShape.Sells, &vega.LiquidityOrder{
			Reference:  refStringToEnum(sell.Reference),
			Proportion: sell.Proportion,
			Offset:     sell.Offset,
		})
	}
	s.ShorteningShape = shorteningShape

	for _, buy := range details.LongeningShape.Buys {
		longeningShape.Buys = append(longeningShape.Buys, &vega.LiquidityOrder{
			Reference:  refStringToEnum(buy.Reference),
			Proportion: buy.Proportion,
			Offset:     buy.Offset,
		})
	}
	for _, sell := range details.LongeningShape.Sells {
		longeningShape.Sells = append(longeningShape.Sells, &vega.LiquidityOrder{
			Reference:  refStringToEnum(sell.Reference),
			Proportion: sell.Proportion,
			Offset:     sell.Offset,
		})
	}
	s.LongeningShape = longeningShape

	s.PosManagementSleepMilliseconds = uint64(details.PosManagementSleepMilliseconds)
	if s.PosManagementSleepMilliseconds < 100 {
		errs = multierror.Append(errs, fmt.Errorf(errInvalid, "PosManagementSleepMilliseconds", errors.New("must be >=100")))
	}

	s.MarketPriceSteeringRatePerSecond = details.MarketPriceSteeringRatePerSecond
	if s.MarketPriceSteeringRatePerSecond <= 0.0 {
		errs = multierror.Append(errs, fmt.Errorf(errInvalid, "MarketPriceSteeringRatePerSecond", errors.New("must be >0")))
	} else if s.MarketPriceSteeringRatePerSecond > 10.0 {
		errs = multierror.Append(errs, fmt.Errorf(errInvalid, "MarketPriceSteeringRatePerSecond", errors.New("must be <=10")))
	}

	s.PriceSteerOrderScale = details.PriceSteerOrderScale
	s.MinPriceSteerFraction = details.MinPriceSteerFraction
	s.LimitOrderDistributionParams = &LODParamsConfig{}
	sm, err := steeringMethodToEnum(details.LimitOrderDistributionParams.Method)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	s.LimitOrderDistributionParams.Method = sm
	s.LimitOrderDistributionParams.GttLength = details.LimitOrderDistributionParams.GttLength
	s.LimitOrderDistributionParams.NumIdenticalBots = details.LimitOrderDistributionParams.NumIdenticalBots
	s.LimitOrderDistributionParams.NumTicksFromMid = details.LimitOrderDistributionParams.NumTicksFromMid
	s.LimitOrderDistributionParams.TgtTimeHorizonHours = details.LimitOrderDistributionParams.TgtTimeHorizonHours

	s.TargetLNVol = details.TargetLNVol
	err = errs.ErrorOrNil()
	return s, err
}

func (s *Strategy) String() string {
	return fmt.Sprintf("normal.Strategy{ExpectedMarkPrice=%d, AuctionVolume=%d, MaxLong=%d, MaxShort=%d, PosManagementFraction=%f, StakeFraction=%f, OrdersFraction=%f, ShorteningShape=TBD(*ShapeConfig), LongeningShape=TBD(*ShapeConfig), PosManagementSleepMilliseconds=%d, MarketPriceSteeringRatePerSecond=%f, MinPriceSteerFraction=%f, PriceSteerOrderScale=%f, LimitOrderDistributionParams=TBD(*LODParamsConfig), TargetLNVol=%f}",
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
