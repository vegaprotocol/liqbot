package normal

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/vega/protos/vega"
)

func (b *bot) runPositionManagement(ctx context.Context) {
	defer b.log.Warning("PositionManagement: Stopped")

	sleepTime := time.Duration(b.config.StrategyDetails.PosManagementSleepMilliseconds) * time.Millisecond
	previousOpenVolume := int64(0)

	for {
		select {
		case <-b.pausePosMgmt:
			b.log.Warning("PositionManagement: Paused")
			<-b.pausePosMgmt
			b.log.Info("PositionManagement: Resumed")
		case <-b.stopPosMgmt:
			return
		case <-ctx.Done():
			b.log.WithFields(log.Fields{
				"error": ctx.Err(),
			}).Warning("PositionManagement: Stopped by context")
			return
		default:
			err := doze(sleepTime, b.stopPosMgmt)
			if err != nil {
				return
			}

			if err := b.EnsureCommitmentAmount(ctx); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to update commitment amount")
			}

			if !b.CanPlaceOrders() {
				if err := b.SeedOrders(ctx); err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to seed orders")
					return
				}
				continue
			}

			previousOpenVolume, err = b.manageDirection(ctx, previousOpenVolume)
			if err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to change LP direction")
			}

			if err = b.managePosition(ctx); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to manage position")
			}
		}
	}
}

func (b *bot) manageDirection(ctx context.Context, previousOpenVolume int64) (int64, error) {
	openVolume := b.Market().OpenVolume()
	buyShape, sellShape, shape := b.GetShape()
	from := "PositionManagement"

	b.log.WithFields(log.Fields{
		"openVolume":         b.Market().OpenVolume(),
		"previousOpenVolume": previousOpenVolume,
		"shape":              shape,
	}).Debug(from + ": Checking for direction change")

	// If we flipped then send the new LP order
	if !((openVolume > 0 && previousOpenVolume <= 0) || (openVolume < 0 && previousOpenVolume >= 0)) {
		return previousOpenVolume, nil
	}

	b.log.WithFields(log.Fields{"shape": shape}).Debug(from + ": Flipping LP direction")

	if err := b.SendLiquidityProvisionAmendment(ctx, nil, buyShape, sellShape); err != nil {
		return openVolume, fmt.Errorf("failed to send liquidity provision amendment: %w", err)
	}

	return openVolume, nil
}

func (b *bot) managePosition(ctx context.Context) error {
	size, side, shouldPlace := b.CheckPosition()
	b.log.WithFields(log.Fields{
		"currentPrice":    b.Market().MarkPrice().String(),
		"balance.General": cache.General(b.Balance(ctx)).String(),
		"balance.Margin":  cache.Margin(b.Balance(ctx)).String(),
		"balance.Bond":    cache.Bond(b.Balance(ctx)).String(),
		"openVolume":      b.Market().OpenVolume(),
		"size":            size,
		"side":            side.String(),
		"shouldPlace":     shouldPlace,
	}).Debug("PositionManagement: Checking for position management")

	if !shouldPlace {
		return nil
	}

	order := &vega.Order{
		MarketId:    b.marketID,
		Size:        size,
		Price:       num.Zero().String(),
		Side:        side,
		TimeInForce: vega.Order_TIME_IN_FORCE_IOC,
		Type:        vega.Order_TYPE_MARKET,
		Reference:   "PosManagement",
	}

	if err := b.SubmitOrder(ctx, order, "PositionManagement", int64(b.config.StrategyDetails.LimitOrderDistributionParams.GttLength)); err != nil {
		return fmt.Errorf("failed to place order: %w", err)
	}

	return nil
}
