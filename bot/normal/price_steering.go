package normal

import (
	"context"
	"fmt"
	"math"
	"time"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	"code.vegaprotocol.io/protos/vega"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types/num"
)

func (b *bot) runPriceSteering(ctx context.Context) {
	defer b.log.Warning("PriceSteering: Stopped")

	if !b.canPlaceOrders() {
		b.log.WithFields(log.Fields{
			"PriceSteerOrderScale": b.config.StrategyDetails.PriceSteerOrderScale,
		}).Debug("PriceSteering: Cannot place orders")

		if err := b.seedOrders(ctx, "PriceSteering"); err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).Error("PriceSteering: Failed to seed orders")
			return
		}
	}

	sleepTime := 1000.0 / b.config.StrategyDetails.MarketPriceSteeringRatePerSecond

	for {
		select {
		case <-b.pausePriceSteer:
			b.log.Warning("PriceSteering: Paused")
			<-b.pausePriceSteer
			b.log.Info("Price steering resumed")
		case <-b.stopPriceSteer:
			return
		case <-ctx.Done():
			b.log.WithFields(log.Fields{
				"error": ctx.Err(),
			}).Warning("PriceSteering: Stopped by context")
			return
		default:
			if err := doze(time.Duration(sleepTime)*time.Millisecond, b.stopPriceSteer); err != nil {
				return
			}

			err := b.steerPrice(ctx)
			if err != nil {
				b.log.WithFields(log.Fields{
					"error":     err.Error(),
					"sleepTime": sleepTime,
				}).Warning("PriceSteering: Error during price steering")
			}

			sleepTime = b.moveSteerSleepTime(sleepTime, err != nil)
		}
	}
}

func (b *bot) steerPrice(ctx context.Context) error {
	externalPrice, err := b.getExternalPrice()
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

	b.log.WithFields(log.Fields{
		"currentPrice":          staticMidPrice.String(),
		"externalPrice":         externalPrice.String(),
		"diff":                  currentDiff.String(),
		"currentDiffFraction":   currentDiffFraction.String(),
		"minPriceSteerFraction": minPriceSteerFraction.String(),
		"shouldMove":            map[vega.Side]string{vega.Side_SIDE_BUY: "UP", vega.Side_SIDE_SELL: "DN"}[side],
	}).Debug("PriceSteering: Steering info")

	if !currentDiffFraction.GreaterThan(minPriceSteerFraction) {
		b.log.Debug("PriceSteering: Current difference is not higher than minimum price steering fraction")
		return nil
	}

	// find out what price and size of the order we should place
	price, size, err := b.getRealisticOrderDetails(externalPrice)
	if err != nil {
		b.log.WithFields(log.Fields{"error": err.Error()}).Fatal("PriceSteering: Unable to get realistic order details for price steering")
	}

	if err = b.submitOrder(
		ctx,
		size.Uint64(),
		price,
		side,
		vega.Order_TIME_IN_FORCE_GTT,
		vega.Order_TYPE_LIMIT,
		"PriceSteeringOrder",
		"PriceSteering",
		int64(b.config.StrategyDetails.LimitOrderDistributionParams.GttLength)); err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

	return nil
}

func (b *bot) getExternalPrice() (*num.Uint, error) {
	externalPriceResponse, err := b.pricingEngine.GetPrice(ppconfig.PriceConfig{
		Base:   b.config.InstrumentBase,
		Quote:  b.config.InstrumentQuote,
		Wander: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get external price: %w", err)
	}

	if externalPriceResponse.Price <= 0 {
		return nil, fmt.Errorf("external price is zero")
	}

	externalPrice := externalPriceResponse.Price * math.Pow(10, float64(b.decimalPlaces))
	externalPriceNum := num.NewUint(uint64(externalPrice))
	return externalPriceNum, nil
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
	size := mulFrac(num.NewUint(1), b.config.StrategyDetails.PriceSteerOrderScale, 15)

	return price, size, nil
}

func (b *bot) getDiscreteThreeLevelPrice(externalPrice *num.Uint) (*num.Uint, error) {
	// this converts something like BTCUSD 3912312345 (five decimal places)
	// to 39123.12345 float.
	decimalPlaces := float64(b.decimalPlaces)
	m0 := num.Zero().Div(externalPrice, num.NewUint(uint64(math.Pow(10, decimalPlaces))))
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
