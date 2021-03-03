package normal

import (
	"fmt"

	"code.vegaprotocol.io/liqbot/config"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

const (
	keyExpectedMarkPrice                = "expectedMarkPrice"
	keyAuctionVolume                    = "auctionVolume"
	keyMaxLong                          = "maxLong"
	keyMaxShort                         = "maxShort"
	keyPosManagementFraction            = "posManagementFraction"
	keyStakeFraction                    = "stakeFraction"
	keyOrdersFraction                   = "ordersFraction"
	keyShorteningShape                  = "shorteningShape"
	keyLongeningShape                   = "longeningShape"
	keyPosManagementSleepMilliseconds   = "posManagementSleepMilliseconds"
	keyMarketPriceSteeringRatePerSecond = "marketPriceSteeringRatePerSecond"
	keyLimitOrderDistributionParams     = "limitOrderDistributionParams"
	keyTargetLNVol                      = "targetLNVol"
)

// ShapeConfig is ... TBD. May be replaced with a struct from Vega Core.
type ShapeConfig struct{}

// LODParamsConfig is ... TBD: a little data structure which sets the algo and params for how limits
// orders are generated.
type LODParamsConfig struct{}

// Strategy configures the normal strategy.
type Strategy struct {
	// ExpectedMarkPrice (optional) specifies the expected mark price for a market that may not yet
	// have a mark price. It is used to calculate margin cost of orders meeting liquidity
	// requirement.
	ExpectedMarkPrice uint64

	// AuctionVolume ...
	AuctionVolume uint64

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

	// LimitOrderDistributionParams ...
	LimitOrderDistributionParams *LODParamsConfig

	// TargetLNVol specifies the target log-normal volatility (e.g. 0.5 for 50%).
	TargetLNVol float64
}

func readStrategyConfig(details map[string]string) (s *Strategy, err error) {
	s = &Strategy{}
	errInvalid := "invalid strategy config for %s"

	var errs *multierror.Error

	s.ExpectedMarkPrice, err = config.ReadUint64(details, keyExpectedMarkPrice)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyExpectedMarkPrice)))
	}

	s.AuctionVolume, err = config.ReadUint64(details, keyAuctionVolume)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyAuctionVolume)))
	}

	s.MaxLong, err = config.ReadUint64(details, keyMaxLong)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyMaxLong)))
	}

	s.MaxShort, err = config.ReadUint64(details, keyMaxShort)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyMaxShort)))
	}

	s.PosManagementFraction, err = config.ReadFloat64(details, keyPosManagementFraction)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyPosManagementFraction)))
	}

	s.StakeFraction, err = config.ReadFloat64(details, keyStakeFraction)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyStakeFraction)))
	}

	s.OrdersFraction, err = config.ReadFloat64(details, keyOrdersFraction)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyOrdersFraction)))
	}

	// ShorteningShape TBD

	// LongeningShape TBD

	s.PosManagementSleepMilliseconds, err = config.ReadUint64(details, keyPosManagementSleepMilliseconds)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyPosManagementSleepMilliseconds)))
	}

	s.MarketPriceSteeringRatePerSecond, err = config.ReadFloat64(details, keyMarketPriceSteeringRatePerSecond)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyMarketPriceSteeringRatePerSecond)))
	}

	// LimitOrderDistributionParams TBD

	s.TargetLNVol, err = config.ReadFloat64(details, keyTargetLNVol)
	if err != nil {
		errs = multierror.Append(errs, errors.Wrap(err, fmt.Sprintf(errInvalid, keyTargetLNVol)))
	}

	err = errs.ErrorOrNil()
	return
}

func (s *Strategy) String() string {
	return fmt.Sprintf("normal.Strategy{ExpectedMarkPrice=%d, AuctionVolume=%d, MaxLong=%d, MaxShort=%d, PosManagementFraction=%f, StakeFraction=%f, OrdersFraction=%f, ShorteningShape=TBD(*ShapeConfig), LongeningShape=TBD(*ShapeConfig), PosManagementSleepMilliseconds=%d, MarketPriceSteeringRatePerSecond=%f, LimitOrderDistributionParams=TBD(*LODParamsConfig), TargetLNVol=%f}",
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
		// s.LimitOrderDistributionParams,
		s.TargetLNVol,
	)
}
