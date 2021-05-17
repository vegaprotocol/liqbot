package normal

import (
	"fmt"
	"strconv"
	"strings"

	"code.vegaprotocol.io/liqbot/config"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

// ShapeConfig is the top level definition of a liquidity shape
type ShapeConfig struct {
	Sells []*proto.LiquidityOrder
	Buys  []*proto.LiquidityOrder
}

// LODParamsConfig is a little data structure which sets the algo and params for how limits
// orders are generated.
type LODParamsConfig struct {
	Method             string
	GttLength          uint64
	TgtTimeHorizon     uint64
	NumTicksFromMid    uint64
	TgtOrdersPerSecond float64
	NumIdenticalBots   int
}

// Strategy configures the normal strategy.
type Strategy struct {
	// ExpectedMarkPrice (optional) specifies the expected mark price for a market that may not yet
	// have a mark price. It is used to calculate margin cost of orders meeting liquidity
	// requirement.
	ExpectedMarkPrice uint64

	// AuctionVolume ...
	AuctionVolume uint64

	// CommitmentFraction is the fractional amount of stake for the LP
	CommitmentFraction float64

	// Fee is the 0->1 fee for supplying liquidity
	Fee float64

	// MaxLong specifies the maximum long position that the bot will tolerate.
	MaxLong uint64

	// MaxShort specifies the maximum short position that the bot will tolerate.
	MaxShort uint64

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

	// PriceSteerOrderSize is the size of a steering order when placed
	PriceSteerOrderSize uint64

	// LimitOrderDistributionParams ...
	LimitOrderDistributionParams *LODParamsConfig

	// TargetLNVol specifies the target log-normal volatility (e.g. 0.5 for 50%).
	TargetLNVol float64
}

func refStringToEnum(reference string) proto.PeggedReference {
	reference = strings.ToUpper(reference)
	switch reference {
	case "ASK":
		return proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK
	case "BID":
		return proto.PeggedReference_PEGGED_REFERENCE_BEST_BID
	case "MID":
		return proto.PeggedReference_PEGGED_REFERENCE_MID
	default:
		return proto.PeggedReference_PEGGED_REFERENCE_UNSPECIFIED
	}
}

func validateStrategyConfig(details config.Strategy) (s *Strategy, err error) {
	s = &Strategy{}
	errInvalid := "invalid strategy config for %s"

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
		Sells: []*proto.LiquidityOrder{},
		Buys:  []*proto.LiquidityOrder{},
	}

	var longeningShape *ShapeConfig = &ShapeConfig{
		Sells: []*proto.LiquidityOrder{},
		Buys:  []*proto.LiquidityOrder{},
	}

	for _, buy := range details.ShorteningShape.Buys {
		shorteningShape.Buys = append(shorteningShape.Buys, &proto.LiquidityOrder{Reference: refStringToEnum(buy.Reference),
			Proportion: buy.Proportion,
			Offset:     buy.Offset,
		})
	}
	for _, sell := range details.ShorteningShape.Sells {
		shorteningShape.Sells = append(shorteningShape.Sells, &proto.LiquidityOrder{Reference: refStringToEnum(sell.Reference),
			Proportion: sell.Proportion,
			Offset:     sell.Offset,
		})
	}
	s.ShorteningShape = shorteningShape

	for _, buy := range details.LongeningShape.Buys {
		longeningShape.Buys = append(longeningShape.Buys, &proto.LiquidityOrder{Reference: refStringToEnum(buy.Reference),
			Proportion: buy.Proportion,
			Offset:     buy.Offset,
		})
	}
	for _, sell := range details.LongeningShape.Sells {
		longeningShape.Sells = append(longeningShape.Sells, &proto.LiquidityOrder{Reference: refStringToEnum(sell.Reference),
			Proportion: sell.Proportion,
			Offset:     sell.Offset,
		})
	}
	s.LongeningShape = longeningShape

	s.PosManagementSleepMilliseconds = uint64(details.PosManagementSleepMilliseconds)
	if s.PosManagementSleepMilliseconds < 100 {
		errs = multierror.Append(errs, errors.Wrap(fmt.Errorf("must be >=100"), fmt.Sprintf(errInvalid, "PosManagementSleepMilliseconds")))
	}

	s.MarketPriceSteeringRatePerSecond = details.MarketPriceSteeringRatePerSecond
	if s.MarketPriceSteeringRatePerSecond <= 0.0 {
		errs = multierror.Append(errs, errors.Wrap(fmt.Errorf("must be >0"), fmt.Sprintf(errInvalid, "MarketPriceSteeringRatePerSecond")))
	} else if s.MarketPriceSteeringRatePerSecond > 10.0 {
		errs = multierror.Append(errs, errors.Wrap(fmt.Errorf("must be <=10"), fmt.Sprintf(errInvalid, "MarketPriceSteeringRatePerSecond")))
	}

	s.PriceSteerOrderSize = details.PriceSteerOrderSize
	s.MinPriceSteerFraction = details.MinPriceSteerFraction
	s.LimitOrderDistributionParams = &LODParamsConfig{}
	s.LimitOrderDistributionParams.Method = details.LimitOrderDistributionParams.Method
	s.LimitOrderDistributionParams.GttLength = details.LimitOrderDistributionParams.GttLength
	s.LimitOrderDistributionParams.NumIdenticalBots = details.LimitOrderDistributionParams.NumIdenticalBots
	s.LimitOrderDistributionParams.NumTicksFromMid = details.LimitOrderDistributionParams.NumTicksFromMid
	s.LimitOrderDistributionParams.TgtOrdersPerSecond = details.LimitOrderDistributionParams.TgtOrdersPerSecond
	s.LimitOrderDistributionParams.TgtTimeHorizon = details.LimitOrderDistributionParams.TgtTimeHorizon

	s.TargetLNVol = details.TargetLNVol
	err = errs.ErrorOrNil()
	return
}

func (s *Strategy) String() string {
	return fmt.Sprintf("normal.Strategy{ExpectedMarkPrice=%d, AuctionVolume=%d, MaxLong=%d, MaxShort=%d, PosManagementFraction=%f, StakeFraction=%f, OrdersFraction=%f, ShorteningShape=TBD(*ShapeConfig), LongeningShape=TBD(*ShapeConfig), PosManagementSleepMilliseconds=%d, MarketPriceSteeringRatePerSecond=%f, MinPriceSteerFraction=%f, PriceSteerOrderSize=%d, LimitOrderDistributionParams=TBD(*LODParamsConfig), TargetLNVol=%f}",
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
		s.PriceSteerOrderSize,
		// s.LimitOrderDistributionParams,
		s.TargetLNVol,
	)
}
