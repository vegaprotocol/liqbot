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

	if err := b.prePositionManagement(ctx); err != nil {
		b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to init position management")
		return
	}

	sleepTime := time.Duration(b.config.StrategyDetails.PosManagementSleepMilliseconds) * time.Millisecond
	previousOpenVolume := int64(0)

	for {
		select {
		case <-b.stopPosMgmt:
			return
		case <-ctx.Done():
			b.log.Warning(ctx.Err())
			return
		default:
			if err := doze(sleepTime, b.stopPosMgmt); err != nil {
				return
			}

			previousOpenVolume = b.managePosition(ctx, previousOpenVolume)
		}
	}
}

func (b *bot) managePosition(ctx context.Context, previousOpenVolume int64) int64 {
	openVolume := b.data.OpenVolume()
	// We always cache off with longening shapes
	buyShape, sellShape, shape := b.getShape()
	// If we flipped then send the new LP order
	if b.shouldAmend(previousOpenVolume) {
		b.log.WithFields(log.Fields{"shape": shape}).Debug("Flipping LP direction")

		if err := b.sendLiquidityProvisionAmendment(ctx, buyShape, sellShape); err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to send liquidity provision")
		} else {
			previousOpenVolume = openVolume
		}
	}

	b.log.WithFields(log.Fields{
		"currentPrice":   b.data.MarkPrice(), // TODO: ?
		"balanceGeneral": b.data.Balance().General,
		"balanceMargin":  b.data.Balance().Margin,
		"openVolume":     b.data.OpenVolume(),
		"shape":          shape,
	}).Debug("Position management info")

	size, side, shouldPlace := b.checkPositionManagement()
	if !shouldPlace {
		return previousOpenVolume
	}

	if err := b.submitOrder(
		ctx,
		size,
		num.Zero(),
		side,
		vega.Order_TIME_IN_FORCE_IOC,
		vega.Order_TYPE_MARKET,
		"PosManagement",
		0,
	); err != nil {
		b.log.WithFields(log.Fields{"error": err, "side": side, "size": size}).Warning("Failed to place a position management order")
	}

	return previousOpenVolume
}

func (b *bot) shouldAmend(previousOpenVolume int64) bool {
	return (b.data.OpenVolume() > 0 && previousOpenVolume <= 0) || (b.data.OpenVolume() < 0 && previousOpenVolume >= 0)
}

func (b *bot) checkPositionManagement() (uint64, vega.Side, bool) {
	var (
		size uint64
		side vega.Side
	)

	shouldPlace := true
	openVolume := b.data.OpenVolume()

	if openVolume >= 0 && num.NewUint(uint64(openVolume)).GT(b.config.StrategyDetails.MaxLong.Get()) {
		size = mulFrac(num.NewUint(uint64(openVolume)), b.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_SELL
	} else if openVolume < 0 && num.NewUint(uint64(-openVolume)).GT(b.config.StrategyDetails.MaxShort.Get()) {
		size = mulFrac(num.NewUint(uint64(-openVolume)), b.config.StrategyDetails.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_BUY
	} else {
		shouldPlace = false
	}

	return size, side, shouldPlace
}

func (b *bot) prePositionManagement(ctx context.Context) error {
	// We always cache off with longening shapes
	buyShape, sellShape, _ := b.getShape()
	// At the cache of each loop, wait for positive general account balance. This is in case the network has
	// been restarted.
	if err := b.checkInitialMargin(buyShape, sellShape); err != nil {
		return fmt.Errorf("failed initial margin check: %w", err)
	}
	// Submit LP order to market.
	if err := b.sendLiquidityProvision(ctx, buyShape, sellShape); err != nil {
		return fmt.Errorf("failed to send liquidity provision order: %w", err)
	}
	// Only update liquidity and position if we are not in auction
	// Check if we can already place orders or if we have a currentPrice we can use
	if b.canPlaceOrders() || b.data.MarkPrice().IsZero() { // TODO: which price?
		return nil
	}

	if err := b.placeAuctionOrders(ctx); err != nil {
		return fmt.Errorf("failed to place auction orders: %v", err)
	}

	return nil
}

func (b *bot) getShape() ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string) {
	var (
		shape     string
		buyShape  []*vega.LiquidityOrder
		sellShape []*vega.LiquidityOrder
	)

	if b.data.OpenVolume() <= 0 {
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

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *bot) placeAuctionOrders(ctx context.Context) error {
	b.log.WithFields(log.Fields{"currentPrice": b.data.MarkPrice()}).Debug("Placing auction orders")

	// Place the random orders split into
	totalVolume := num.Zero()

	rand.Seed(time.Now().UnixNano())

	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		remaining := num.Zero().Sub(b.config.StrategyDetails.AuctionVolume.Get(), totalVolume)
		size := num.Min(num.UintChain(b.config.StrategyDetails.AuctionVolume.Get()).Div(num.NewUint(10)).Add(num.NewUint(1)).Get(), remaining)
		// #nosec G404
		price := num.Zero().Add(b.data.MarkPrice(), num.NewUint(uint64(rand.Int63n(100)-50)))
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
			return fmt.Errorf("failed to send auction order: %w", err)
		}

		totalVolume = num.Zero().Add(totalVolume, size)
	}

	b.log.WithFields(log.Fields{"totalVolume": totalVolume}).Debug("Placed auction orders")

	return nil
}

func (b *bot) checkInitialMargin(buyShape, sellShape []*vega.LiquidityOrder) error {
	// Turn the shapes into a set of orders scaled by commitment
	obligation := b.getCommitment()
	buyOrders := b.calculateOrderSizes(obligation, buyShape)
	sellOrders := b.calculateOrderSizes(obligation, sellShape)

	buyRisk := 0.01
	sellRisk := 0.01

	buyCost := b.calculateMarginCost(buyRisk, buyOrders)
	sellCost := b.calculateMarginCost(sellRisk, sellOrders)

	shapeMarginCost := num.Max(buyCost, sellCost)
	avail := mulFrac(b.data.Balance().General, b.config.StrategyDetails.OrdersFraction, 15)

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

func (b *bot) getCommitment() *num.Uint {
	// CommitmentAmount is the fractional commitment value * total collateral
	return mulFrac(b.data.Balance().Total(), b.config.StrategyDetails.CommitmentFraction, 15)
}

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice).
func (b *bot) calculateOrderSizes(obligation *num.Uint, liquidityOrders []*vega.LiquidityOrder) []*vega.Order {
	orders := make([]*vega.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	totalProportion := num.Zero()
	for _, order := range liquidityOrders {
		totalProportion.Add(totalProportion, num.NewUint(uint64(order.Proportion)))
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		size := num.UintChain(obligation.Clone()).
			Mul(num.NewUint(uint64(lo.Proportion))).
			Mul(num.NewUint(10)).
			Div(totalProportion).Div(b.data.MarkPrice()).Get()
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
func (b *bot) calculateMarginCost(risk float64, orders []*vega.Order) *num.Uint {
	margins := make([]*num.Uint, len(orders))

	for i, order := range orders {
		if order.Side == vega.Side_SIDE_BUY {
			margins[i] = num.NewUint(1 + order.Size)
		} else {
			margins[i] = num.NewUint(order.Size)
		}
	}

	totalMargin := num.UintChain(num.Zero()).Add(margins...).Mul(b.data.MarkPrice()).Get()
	return mulFrac(totalMargin, risk, 15)
}

func (b *bot) sendLiquidityProvision(ctx context.Context, buys, sells []*vega.LiquidityOrder) error {
	commitment := b.getCommitment()

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey: b.walletPubKey,
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
			LiquidityProvisionSubmission: &commandspb.LiquidityProvisionSubmission{
				Fee:              b.config.StrategyDetails.Fee,
				MarketId:         b.marketID,
				CommitmentAmount: commitment.String(),
				Buys:             buys,
				Sells:            sells,
			},
		},
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       b.data.Balance().Total(),
	}).Debug("Submitted LiquidityProvisionSubmission")
	return nil
}

// call this if the position flips
func (b *bot) sendLiquidityProvisionAmendment(ctx context.Context, buys, sells []*vega.LiquidityOrder) error {
	commitment := b.getCommitment()
	if commitment == num.Zero() {
		return b.sendLiquidityProvisionCancellation(ctx)
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey: b.walletPubKey,
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
			LiquidityProvisionAmendment: &commandspb.LiquidityProvisionAmendment{
				MarketId:         b.marketID,
				CommitmentAmount: commitment.String(),
				Fee:              b.config.StrategyDetails.Fee,
				Sells:            sells,
				Buys:             buys,
			},
		},
	}

	if _, err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       b.data.Balance().Total(),
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *bot) sendLiquidityProvisionCancellation(ctx context.Context) error {
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
		"commitment":         num.Zero(),
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       b.data.Balance().Total(),
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func mulFrac(n *num.Uint, x float64, precision float64) *num.Uint {
	val := num.NewUint(uint64(x * math.Pow(10, precision)))
	val.Mul(val, n)
	val.Div(val, num.NewUint(uint64(math.Pow(10, precision))))
	return val
}
