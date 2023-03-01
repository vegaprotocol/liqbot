package normal

import (
	"context"
	"fmt"
	"math"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/vega/logging"
	"code.vegaprotocol.io/vega/protos/vega"
)

func (b *bot) runPriceSteering(ctx context.Context) {
	defer b.log.Warn("PriceSteering: Stopped")

	sleepTime := 1000.0 / b.config.StrategyDetails.MarketPriceSteeringRatePerSecond

	for {
		select {
		case <-b.pausePriceSteer:
			b.log.Warn("PriceSteering: Paused")
			<-b.pausePriceSteer
			b.log.Info("Price steering resumed")
		case <-b.stopPriceSteer:
			return
		case <-ctx.Done():
			b.log.Warn("PriceSteering: Stopped by context")
			return
		default:
			if err := doze(time.Duration(sleepTime)*time.Millisecond, b.stopPriceSteer); err != nil {
				return
			}

			err := b.steerPrice(ctx)
			if err != nil {
				b.log.With(
					logging.Float64("sleepTime", sleepTime)).
					Warn("PriceSteering: Error during price steering", logging.Error(err))
			}

			sleepTime = b.moveSteerSleepTime(sleepTime, err != nil)
		}
	}
}

func (b *bot) steerPrice(ctx context.Context) error {
	externalPrice, err := b.GetExternalPrice()
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	staticMidPrice := b.Market().StaticMidPrice()
	currentDiff, statIsGt := num.Zero().Delta(externalPrice, staticMidPrice)
	currentDiffFraction := num.DecimalFromUint(currentDiff).Div(num.DecimalFromUint(externalPrice))
	minPriceSteerFraction := num.DecimalFromInt64(int64(100)).Mul(num.DecimalFromFloat(b.config.StrategyDetails.MinPriceSteerFraction))

	side := vega.Side_SIDE_BUY
	if statIsGt {
		side = vega.Side_SIDE_SELL
	}

	b.log.With(
		logging.String("currentPrice", staticMidPrice.String()),
		logging.String("externalPrice", externalPrice.String()),
		logging.String("diff", currentDiff.String()),
		logging.String("currentDiffFraction", currentDiffFraction.String()),
		logging.String("minPriceSteerFraction", minPriceSteerFraction.String()),
		logging.String("shouldMove", map[vega.Side]string{vega.Side_SIDE_BUY: "UP", vega.Side_SIDE_SELL: "DN"}[side]),
	).Debug("PriceSteering: Steering info")

	if !currentDiffFraction.GreaterThan(minPriceSteerFraction) {
		b.log.Debug("PriceSteering: Current difference is not higher than minimum price steering fraction")
		return nil
	}

	// find out what price and size of the order we should place
	price, size, err := b.getRealisticOrderDetails(externalPrice)
	if err != nil {
		return fmt.Errorf("PriceSteering: Unable to get realistic order details for price steering: %w", err)
	}

	order := &vega.Order{
		MarketId:    b.marketID,
		Size:        size.Uint64(),
		Price:       price.String(),
		Side:        side,
		TimeInForce: vega.Order_TIME_IN_FORCE_GTT,
		Type:        vega.Order_TYPE_LIMIT,
		Reference:   "PriceSteeringOrder",
		// ExpiresAt:   time.Now().Add(10 * time.Minute).Unix(), TODO
	}

	expectedBalance := price.Mul(price, size)

	if err := b.EnsureBalance(ctx, b.settlementAsset, cache.General, expectedBalance, b.decimalPlaces, b.config.StrategyDetails.TopUpScale, "PriceSteering"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	if err = b.SubmitOrder(
		ctx, order, "PriceSteering",
		int64(b.config.StrategyDetails.LimitOrderDistributionParams.GttLengthSeconds)); err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

	return nil
}

// getRealisticOrderDetails uses magic to return a realistic order price and size.
func (b *bot) getRealisticOrderDetails(externalPrice *num.Uint) (*num.Uint, *num.Uint, error) {
	var price *num.Uint

	switch b.config.StrategyDetails.LimitOrderDistributionParams.Method {
	case config.DiscreteThreeLevel:
		var err error
		price, err = b.getDiscreteThreeLevelPrice(externalPrice)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get discrete three level price: %w", err)
		}
	case config.CoinAndBinomial:
		price = externalPrice
	default:
		return nil, nil, fmt.Errorf("method for generating price distributions not recognised")
	}
	// TODO: size?
	size := num.MulFrac(num.NewUint(1), b.config.StrategyDetails.PriceSteerOrderScale, 15)

	return price, size, nil
}

func (b *bot) getDiscreteThreeLevelPrice(externalPrice *num.Uint) (*num.Uint, error) {
	// this converts something like BTCUSD 3912312345 (five decimal places)
	// to 39123.12345 float.
	if uint64(len(externalPrice.String())) < b.decimalPlaces {
		return nil, fmt.Errorf("external price has fewer digits than the market decimal places")
	}

	decimalPlaces := float64(b.decimalPlaces)

	m0 := num.Zero().Div(externalPrice, num.NewUint(uint64(math.Pow(10, decimalPlaces))))
	if m0.IsZero() {
		return nil, fmt.Errorf("external price is zero")
	}
	tickSize := 1.0 / math.Pow(10, decimalPlaces)
	delta := float64(b.config.StrategyDetails.LimitOrderDistributionParams.NumTicksFromMid) * tickSize
	numOrdersPerSec := b.config.StrategyDetails.MarketPriceSteeringRatePerSecond
	n := 3600 * numOrdersPerSec / b.config.StrategyDetails.LimitOrderDistributionParams.TgtTimeHorizonHours
	tgtTimeHorizonYrFrac := b.config.StrategyDetails.LimitOrderDistributionParams.TgtTimeHorizonHours / 24.0 / 365.25

	priceFloat, err := generatePriceUsingDiscreteThreeLevel(m0.Float64(), delta, b.config.StrategyDetails.TargetLNVol, tgtTimeHorizonYrFrac, n)
	if err != nil {
		return nil, fmt.Errorf("error generating price: %w", err)
	}
	// we need to add back decimals
	return num.NewUint(uint64(math.Round(priceFloat * math.Pow(10, decimalPlaces)))), nil
}

func (b *bot) moveSteerSleepTime(sleepTime float64, up bool) float64 {
	if up && sleepTime < 29000 {
		return sleepTime + 1000
	}
	return 1000.0 / b.config.StrategyDetails.MarketPriceSteeringRatePerSecond
}
