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
		case <-b.pausePosMgmt:
			b.log.Warning("Position management paused")
			<-b.pausePosMgmt
			b.log.Info("Position management resumed")
		case <-b.stopPosMgmt:
			return
		case <-ctx.Done():
			b.log.WithFields(log.Fields{
				"error": ctx.Err(),
			}).Warning("Stopped by context")
			return
		default:
			err := doze(sleepTime, b.stopPosMgmt)
			if err != nil {
				return
			}

			// Only update liquidity and position if we are not in auction
			if !b.canPlaceOrders() {
				if err = b.placeAuctionOrders(ctx); err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to place auction orders")
				}
				continue
			}

			previousOpenVolume, err = b.manageDirection(ctx, previousOpenVolume)
			if err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to change LP direction")
			}

			if err = b.managePosition(ctx); err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to manage position")
			}
		}
	}
}

func (b *bot) manageDirection(ctx context.Context, previousOpenVolume int64) (int64, error) {
	openVolume := b.data.OpenVolume()
	buyShape, sellShape, shape := b.getShape()

	b.log.WithFields(log.Fields{
		"openVolume":         b.data.OpenVolume(),
		"previousOpenVolume": previousOpenVolume,
		"shape":              shape,
	}).Debug("Checking for direction change")

	// If we flipped then send the new LP order
	if !b.shouldAmend(previousOpenVolume) {
		return openVolume, nil
	}

	b.log.WithFields(log.Fields{"shape": shape}).Debug("Flipping LP direction")

	if err := b.sendLiquidityProvisionAmendment(ctx, buyShape, sellShape); err != nil {
		return openVolume, fmt.Errorf("failed to send liquidity provision amendment: %w", err)
	}

	return openVolume, nil
}

func (b *bot) shouldAmend(previousOpenVolume int64) bool {
	return (b.data.OpenVolume() > 0 && previousOpenVolume <= 0) || (b.data.OpenVolume() < 0 && previousOpenVolume >= 0)
}

func (b *bot) managePosition(ctx context.Context) error {
	size, side, shouldPlace := b.checkPosition()
	if !shouldPlace {
		return nil
	}

	b.log.WithFields(log.Fields{
		"currentPrice":   b.data.MarkPrice(),
		"balanceGeneral": b.data.Balance().General,
		"balanceMargin":  b.data.Balance().Margin,
		"openVolume":     b.data.OpenVolume(),
	}).Debug("Position management info")

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
		return fmt.Errorf("failed to place order: %w", err)
	}

	return nil
}

func (b *bot) checkPosition() (uint64, vega.Side, bool) {
	size := uint64(0)
	side := vega.Side_SIDE_UNSPECIFIED
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

	return nil
}

func (b *bot) getShape() ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string) {
	// We always cache off with longening shapes
	shape := "longening"
	buyShape := b.config.StrategyDetails.LongeningShape.Buys.ToVegaLiquidityOrders()
	sellShape := b.config.StrategyDetails.LongeningShape.Sells.ToVegaLiquidityOrders()

	if b.data.OpenVolume() > 0 {
		shape = "shortening"
		buyShape = b.config.StrategyDetails.ShorteningShape.Buys.ToVegaLiquidityOrders()
		sellShape = b.config.StrategyDetails.ShorteningShape.Sells.ToVegaLiquidityOrders()
	}

	return buyShape, sellShape, shape
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

	if !avail.LT(shapeMarginCost) {
		return nil
	}

	missingPercent := "Inf"

	if !avail.IsZero() {
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

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *bot) placeAuctionOrders(ctx context.Context) error {
	// Check if we have a currentPrice we can use
	if b.data.MarkPrice().IsZero() {
		b.log.Debug("No current price to place auction orders")
		return nil
	}

	b.log.WithFields(log.Fields{"currentPrice": b.data.MarkPrice()}).Debug("Placing auction orders")

	// Place the random orders split into
	totalVolume := num.Zero()

	rand.Seed(time.Now().UnixNano())

	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		time.Sleep(time.Second * 2)

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
				MarketId:         b.marketID,
				CommitmentAmount: commitment.String(),
				Fee:              b.config.StrategyDetails.Fee,
				Sells:            sells,
				Buys:             buys,
			},
		},
	}

	if err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
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

	if err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.config.StrategyDetails.CommitmentFraction,
		"balanceTotal":       b.data.Balance().Total(),
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *bot) getCommitment() *num.Uint {
	// CommitmentAmount is the fractional commitment value * total collateral
	return mulFrac(b.data.Balance().Total(), b.config.StrategyDetails.CommitmentFraction, 15)
}

func (b *bot) sendLiquidityProvisionCancellation(ctx context.Context) error {
	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey: b.walletPubKey,
		Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionCancellation{
			LiquidityProvisionCancellation: &commandspb.LiquidityProvisionCancellation{
				MarketId: b.marketID,
			},
		},
	}

	if err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
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
