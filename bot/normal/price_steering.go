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
	defer b.log.Warning("Price steering stopped")

	if !b.canPlaceOrders() {
		if err := b.seedOrders(ctx); err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).Error("failed to seed orders")
			return
		}
	}

	sleepTime := 1000.0 / b.config.StrategyDetails.MarketPriceSteeringRatePerSecond

	for {
		select {
		case <-b.stopPriceSteer:
			b.log.Debug("Stopping bot market price steering")
			return
		case <-ctx.Done():
			b.log.Warning(ctx.Err())
			return
		default:
			if err := doze(time.Duration(sleepTime)*time.Millisecond, b.stopPriceSteer); err != nil {
				b.log.Debug("Stopping bot market price steering")
				return
			}

			if err := b.steerPrice(ctx); err == nil {
				sleepTime = 1000.0 / b.config.StrategyDetails.MarketPriceSteeringRatePerSecond
			} else {
				if sleepTime < 29000 {
					sleepTime += 1000
				}
				b.log.WithFields(log.Fields{
					"error":     err.Error(),
					"sleepTime": sleepTime,
				}).Warning("Error during price steering")
			}
		}

	}
}

func (b *bot) steerPrice(ctx context.Context) error {
	externalPriceResponse, err := b.pricingEngine.GetPrice(ppconfig.PriceConfig{
		Base:   b.config.InstrumentBase,
		Quote:  b.config.InstrumentQuote,
		Wander: true,
	})
	if err != nil {
		return fmt.Errorf("failed to get external price: %w", err)
	}

	decPlaces := num.NewUint(uint64(math.Pow10(b.decimalPlaces)))
	externalPrice := num.NewUint(uint64(externalPriceResponse.Price))
	externalPrice = num.UintChain(externalPrice).Mul(decPlaces).Get()

	marketData := b.data.MarketDataGet()
	if marketData == nil {
		return fmt.Errorf("no market data")
	}

	currentPrice := marketData.PriceStaticMid
	currentDiff := b.getCurrentDiff(externalPrice, currentPrice)

	if !currentDiff.GT(num.NewUint(uint64(100.0 * b.config.StrategyDetails.MinPriceSteerFraction))) {
		return nil
	}

	side := vega.Side_SIDE_SELL
	if externalPrice.GT(currentPrice) {
		side = vega.Side_SIDE_BUY
	}

	// find out what price and size of the order we should place
	price, size, err := b.getRealisticOrderDetails(externalPrice)
	if err != nil {
		b.log.WithFields(log.Fields{"error": err.Error()}).Fatal("Unable to get realistic order details for price steering")
	}

	size = mulFrac(size, b.config.StrategyDetails.PriceSteerOrderScale, 15)

	b.log.WithFields(log.Fields{
		"size":  size,
		"side":  side,
		"price": price,
	}).Debug("Submitting order")

	b.log.WithFields(log.Fields{
		"currentPrice":  currentPrice,
		"externalPrice": externalPrice,
		"diff":          currentDiff,
		"shouldMove":    map[vega.Side]string{vega.Side_SIDE_BUY: "UP", vega.Side_SIDE_SELL: "DN"}[side],
	}).Debug("Steering info")

	secsFromNow := int64(b.config.StrategyDetails.LimitOrderDistributionParams.GttLength)
	ref := "PriceSteeringOrder"

	return b.submitOrder(ctx, size.Uint64(), price, side, vega.Order_TIME_IN_FORCE_GTT, vega.Order_TYPE_LIMIT, ref, secsFromNow)
}

func (b *bot) getCurrentDiff(externalPrice *num.Uint, currentPrice *num.Uint) *num.Uint {
	// We only want to steer the price if the external and market price
	// are greater than a certain percentage apart
	currentDiff := num.Zero().Sub(externalPrice, currentPrice)
	if currentPrice.GT(externalPrice) {
		currentDiff = num.Zero().Sub(currentPrice, externalPrice)
	}
	return num.UintChain(currentDiff).Mul(num.NewUint(100)).Div(externalPrice).Get()
}

// getRealisticOrderDetails uses magic to return a realistic order price and size.
func (b *bot) getRealisticOrderDetails(externalPrice *num.Uint) (*num.Uint, *num.Uint, error) {
	var price *num.Uint

	switch b.config.StrategyDetails.LimitOrderDistributionParams.Method {
	case config.DiscreteThreeLevel:
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
			return nil, nil, fmt.Errorf("error generating price: %w", err)
		}
		// we need to add back decimals
		price = num.NewUint(uint64(math.Round(priceFloat * math.Pow(10, decimalPlaces))))
	case config.CoinAndBinomial:
		price = externalPrice
	default:
		return nil, nil, fmt.Errorf("method for generating price distributions not recognised")
	}

	// TODO: price?
	return price, num.NewUint(1), nil
}
