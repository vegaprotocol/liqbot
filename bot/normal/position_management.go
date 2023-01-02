package normal

import (
	"context"
	"fmt"
	"time"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/vega/logging"
	"code.vegaprotocol.io/vega/protos/vega"
)

func (b *bot) runPositionManagement(ctx context.Context) {
	defer b.log.Warn("PositionManagement: Stopped")

	sleepTime := time.Duration(b.config.StrategyDetails.PosManagementSleepMilliseconds) * time.Millisecond
	previousOpenVolume := int64(0)

	for {
		select {
		case <-b.pausePosMgmt:
			b.log.Warn("PositionManagement: Paused")
			<-b.pausePosMgmt
			b.log.Info("PositionManagement: Resumed")
		case <-b.stopPosMgmt:
			return
		case <-ctx.Done():
			b.log.Warn("PositionManagement: Stopped by context")
			return
		default:
			err := doze(sleepTime, b.stopPosMgmt)
			if err != nil {
				return
			}

			if err := b.EnsureCommitmentAmount(ctx); err != nil {
				b.log.Warn("PositionManagement: Failed to update commitment amount", logging.Error(err))
			}

			if !b.CanPlaceOrders() {
				if err := b.SeedOrders(ctx); err != nil {
					b.log.Warn("PositionManagement: Failed to seed orders", logging.Error(err))
					return
				}
				continue
			}

			previousOpenVolume, err = b.manageDirection(ctx, previousOpenVolume)
			if err != nil {
				b.log.Warn("PositionManagement: Failed to change LP direction", logging.Error(err))
			}

			if err = b.managePosition(ctx); err != nil {
				b.log.Warn("PositionManagement: Failed to manage position", logging.Error(err))
			}
		}
	}
}

func (b *bot) manageDirection(ctx context.Context, previousOpenVolume int64) (int64, error) {
	openVolume := b.Market().OpenVolume()
	buyShape, sellShape, shape := b.GetShape()
	from := "PositionManagement"

	b.log.With(
		logging.Int64("openVolume", b.Market().OpenVolume()),
		logging.Int64("previousOpenVolume", previousOpenVolume),
		logging.String("shape", shape),
	).Debug(from + ": Checking for direction change")

	// If we flipped then send the new LP order
	if !((openVolume > 0 && previousOpenVolume <= 0) || (openVolume < 0 && previousOpenVolume >= 0)) {
		return previousOpenVolume, nil
	}

	b.log.With(logging.String("shape", shape)).Debug(from + ": Flipping LP direction")

	if err := b.SendLiquidityProvisionAmendment(ctx, nil, buyShape, sellShape); err != nil {
		return openVolume, fmt.Errorf("failed to send liquidity provision amendment: %w", err)
	}

	return openVolume, nil
}

func (b *bot) managePosition(ctx context.Context) error {
	size, side, shouldPlace := b.CheckPosition()
	b.log.With(
		logging.String("currentPrice", b.Market().MarkPrice().String()),
		logging.String("balance.General", cache.General(b.Balance(ctx)).String()),
		logging.String("balance.Margin", cache.Margin(b.Balance(ctx)).String()),
		logging.String("balance.Bond", cache.Bond(b.Balance(ctx)).String()),
		logging.Int64("openVolume", b.Market().OpenVolume()),
		logging.Uint64("size", size),
		logging.String("side", side.String()),
		logging.Bool("shouldPlace", shouldPlace),
	).Debug("PositionManagement: Checking for position management")

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

	if err := b.SubmitOrder(ctx, order, "PositionManagement", int64(b.config.StrategyDetails.LimitOrderDistributionParams.GttLengthSeconds)); err != nil {
		return fmt.Errorf("failed to place order: %w", err)
	}

	return nil
}
