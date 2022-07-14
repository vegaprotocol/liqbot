package normal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"code.vegaprotocol.io/protos/vega"
	commandspb "code.vegaprotocol.io/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types/num"
)

func (b *bot) runPositionManagement(ctx context.Context) {
	defer b.log.Warning("Position management stopped")

	sleepTime := b.config.StrategyDetails.PosManagementSleepMilliseconds
	auctionOrdersPlaced := false
	managePosition := b.managePosition()

	for {
		select {
		case <-b.stopPosMgmt:
			b.log.Debug("Stopping bot position management")
			return
		case <-ctx.Done():
			b.log.Warning(ctx.Err())
			return
		default:
			if err := doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).
					Debug("Stopping bot position management")
				return
			}
			// Only update liquidity and position if we are not in auction
			if !b.canPlaceOrders() && !auctionOrdersPlaced {
				b.log.Debug("placing auction orders")
				b.placeAuctionOrders(ctx)
				auctionOrdersPlaced = true
				continue
			}

			managePosition(ctx)
		}
	}
}

func (b *bot) managePosition() func(context.Context) {
	previousOpenVolume := int64(0)
	firstTime := true

	return func(ctx context.Context) {
		balance := b.data.BalanceGet()
		if balance == nil {
			b.log.Warning("No balance available")
			return
		}

		openVolume := b.data.OpenVolumeGet()
		// We always cache off with longening shapes
		buyShape, sellShape, shape := b.getShape(openVolume)
		balanceTotal := num.Sum(balance.General, balance.Margin, balance.Bond)

		marketData := b.data.MarketDataGet()
		if marketData == nil {
			b.log.Warning("No market data available")
			return
		}
		// At the cache of each loop, wait for positive general account balance. This is in case the network has
		// been restarted.
		if firstTime {
			if err := b.checkInitialMargin(buyShape, sellShape, marketData.PriceStaticMid, balance.General, balanceTotal); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Error("Failed initial margin check")
				return
			}
			// Submit LP order to market.
			if err := b.sendLiquidityProvision(ctx, buyShape, sellShape, balanceTotal); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Error("Failed to send liquidity provision order")
			}

			firstTime = false
		}

		buyShape, sellShape, shape = b.getShape(openVolume)
		// If we flipped then send the new LP order
		if shouldAmend(openVolume, previousOpenVolume) {
			b.log.WithFields(log.Fields{"shape": shape}).Debug("Flipping LP direction")

			if err := b.sendLiquidityProvisionAmendment(ctx, buyShape, sellShape, balanceTotal); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to send liquidity provision")
			}
			previousOpenVolume = openVolume
		}

		b.log.WithFields(log.Fields{
			"currentPrice":   marketData.PriceStaticMid,
			"balanceGeneral": balance.General,
			"balanceMargin":  balance.Margin,
			"openVolume":     openVolume,
			"shape":          shape,
		}).Debug("Position management info")

		size, side, shouldPlace := b.checkPositionManagement(openVolume)
		if !shouldPlace {
			return
		}

		ref := "PosManagement"
		tif := vega.Order_TIME_IN_FORCE_IOC
		orderTyp := vega.Order_TYPE_MARKET

		if err := b.submitOrder(ctx, size, num.Zero(), side, tif, orderTyp, ref, 0); err != nil {
			b.log.WithFields(log.Fields{"error": err, "side": side, "size": size}).
				Warning("Failed to place a position management order")
		}
	}
}

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *bot) placeAuctionOrders(ctx context.Context) {
	marketData := b.data.MarketDataGet()
	if marketData == nil {
		b.log.Warning("No market data available")
		return
	}

	currentPrice := marketData.PriceStaticMid

	// Check we have a currentPrice we can use
	if currentPrice.EQUint64(0) {
		return
	}

	// Place the random orders split into
	totalVolume := num.Zero()

	rand.Seed(time.Now().UnixNano())

	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		remaining := num.Zero().Sub(b.config.StrategyDetails.AuctionVolume.Get(), totalVolume)
		size := num.Min(num.UintChain(b.config.StrategyDetails.AuctionVolume.Get()).Div(num.NewUint(10)).Add(num.NewUint(1)).Get(), remaining)
		// #nosec G404
		price := num.Zero().Add(currentPrice, num.NewUint(uint64(rand.Int63n(100)-50)))
		side := vega.Side_SIDE_BUY
		// #nosec G404
		if rand.Intn(2) == 0 {
			side = vega.Side_SIDE_SELL
		}

		tif := vega.Order_TIME_IN_FORCE_GTT
		orderType := vega.Order_TYPE_LIMIT
		ref := "AuctionOrder"

		if err := b.submitOrder(ctx, size.Uint64(), price, side, tif, orderType, ref, 330); err != nil {
			// We failed to send an order so stop trying to send anymore
			break
		}

		totalVolume = num.Zero().Add(totalVolume, size)
	}
}

func (b *bot) checkInitialMargin(buyShape, sellShape []*vega.LiquidityOrder, currentPrice, balanceGeneral, balanceTotal *num.Uint) error {
	// Turn the shapes into a set of orders scaled by commitment
	obligation := mulFrac(balanceTotal, b.config.StrategyDetails.CommitmentFraction, 15)
	buyOrders := b.calculateOrderSizes(obligation, buyShape, currentPrice)
	sellOrders := b.calculateOrderSizes(obligation, sellShape, currentPrice)

	buyRisk := 0.01
	sellRisk := 0.01

	buyCost := b.calculateMarginCost(buyRisk, buyOrders, currentPrice)
	sellCost := b.calculateMarginCost(sellRisk, sellOrders, currentPrice)

	shapeMarginCost := num.Max(buyCost, sellCost)

	avail := mulFrac(balanceGeneral, b.config.StrategyDetails.OrdersFraction, 15)

	if avail.LT(shapeMarginCost) {
		var missingPercent string
		if avail.EQUint64(0) {
			missingPercent = "Inf"
		} else {
			x := num.UintChain(shapeMarginCost).Sub(avail).Mul(num.NewUint(100)).Div(avail).Get()
			missingPercent = fmt.Sprintf("%v%%", x)
		}
		b.log.WithFields(log.Fields{
			"available":      avail,
			"cost":           shapeMarginCost,
			"missing":        num.Zero().Sub(avail, shapeMarginCost),
			"missingPercent": missingPercent,
		}).Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")
		return errors.New("not enough collateral")
	}
	return nil
}

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice).
func (b *bot) calculateOrderSizes(obligation *num.Uint, liquidityOrders []*vega.LiquidityOrder, currentPrice *num.Uint) []*vega.Order {
	orders := make([]*vega.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	totalProportion := num.Zero()
	for _, order := range liquidityOrders {
		totalProportion.Add(totalProportion, num.NewUint(uint64(order.Proportion)))
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		size := num.UintChain(obligation).Mul(num.NewUint(uint64(lo.Proportion))).Mul(num.NewUint(10)).Div(totalProportion).Div(currentPrice).Get()
		peggedOrder := vega.PeggedOrder{
			Reference: lo.Reference,
			Offset:    lo.Offset,
		}

		order := vega.Order{
			MarketId:    b.marketID,
			PartyId:     b.walletPubKey,
			Side:        vega.Side_SIDE_BUY,
			Remaining:   size.Uint64(),
			Size:        size.Uint64(),
			TimeInForce: vega.Order_TIME_IN_FORCE_GTC,
			Type:        vega.Order_TYPE_LIMIT,
			PeggedOrder: &peggedOrder,
		}
		orders = append(orders, &order)
	}
	return orders
}

// calculateMarginCost estimates the margin cost of the set of orders.
func (b *bot) calculateMarginCost(risk float64, orders []*vega.Order, currentPrice *num.Uint) *num.Uint {
	margins := make([]*num.Uint, len(orders))

	for i, order := range orders {
		if order.Side == vega.Side_SIDE_BUY {
			margins[i] = num.NewUint(1 + order.Size)
		} else {
			margins[i] = num.NewUint(order.Size)
		}
	}

	totalMargin := num.UintChain(num.NewUint(0)).Add(margins...).Mul(currentPrice).Get()
	return mulFrac(totalMargin, risk, 15)
}

func (b *bot) sendLiquidityProvision(ctx context.Context, buys, sells []*vega.LiquidityOrder, balanceTotal *num.Uint) error {
	// CommitmentAmount is the fractional commitment value * total collateral
	commitment := mulFrac(balanceTotal, b.config.StrategyDetails.CommitmentFraction, 15)

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
		LiquidityProvisionSubmission: &commandspb.LiquidityProvisionSubmission{
			Fee:              b.config.StrategyDetails.Fee,
			MarketId:         b.marketID,
			CommitmentAmount: commitment.String(),
			Buys:             buys,
			Sells:            sells,
		},
	}
	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission(%v): %w", cmd, err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       balanceTotal,
	}).Debug("Submitted LiquidityProvisionSubmission")
	return nil
}

// call this if the position flips
func (b *bot) sendLiquidityProvisionAmendment(ctx context.Context, buys, sells []*vega.LiquidityOrder, balanceTotal *num.Uint) error {
	commitment := mulFrac(balanceTotal, b.config.StrategyDetails.CommitmentFraction, 15)

	if commitment == num.NewUint(0) {
		return b.sendLiquidityProvisionCancellation(ctx, balanceTotal)
	}

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
		LiquidityProvisionAmendment: &commandspb.LiquidityProvisionAmendment{
			MarketId:         b.marketID,
			CommitmentAmount: commitment.String(),
			Fee:              b.config.StrategyDetails.Fee,
			Sells:            sells,
			Buys:             buys,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       balanceTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *bot) sendLiquidityProvisionCancellation(ctx context.Context, balTotal *num.Uint) error {
	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionCancellation{
		LiquidityProvisionCancellation: &commandspb.LiquidityProvisionCancellation{
			MarketId: b.marketID,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionCancellation: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         num.NewUint(0),
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *bot) getShape(openVolume int64) ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string) {
	var (
		shape     string
		buyShape  []*vega.LiquidityOrder
		sellShape []*vega.LiquidityOrder
	)
	if openVolume <= 0 {
		shape = "longening"
		buyShape = b.config.StrategyDetails.LongeningShape.Buys.ToVegaLiquidityOrders()
		sellShape = b.config.StrategyDetails.LongeningShape.Sells.ToVegaLiquidityOrders()
	} else {
		shape = "shortening"
		buyShape = b.config.StrategyDetails.ShorteningShape.Buys.ToVegaLiquidityOrders()
		sellShape = b.config.StrategyDetails.ShorteningShape.Sells.ToVegaLiquidityOrders()
	}

	return buyShape, sellShape, shape
}

func shouldAmend(openVolume int64, previousOpenVolume int64) bool {
	return (openVolume > 0 && previousOpenVolume <= 0) ||
		(openVolume < 0 && previousOpenVolume >= 0)
}

func (b *bot) checkPositionManagement(openVolume int64) (uint64, vega.Side, bool) {
	var (
		size uint64
		side vega.Side
	)

	shouldSend := true

	if openVolume >= 0 && num.NewUint(uint64(openVolume)).GT(b.config.StrategyDetails.MaxLong.Get()) {
		size = mulFrac(num.NewUint(uint64(openVolume)), b.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_SELL
	} else if openVolume < 0 && num.NewUint(uint64(-openVolume)).GT(b.config.StrategyDetails.MaxShort.Get()) {
		size = mulFrac(num.NewUint(uint64(-openVolume)), b.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_BUY
	} else {
		shouldSend = false
	}

	return size, side, shouldSend
}

func mulFrac(n *num.Uint, x float64, precision float64) *num.Uint {
	val := num.NewUint(uint64(x * math.Pow(10, precision)))
	val.Mul(val, n)
	val.Div(val, num.NewUint(uint64(math.Pow(10, precision))))
	return val
}
