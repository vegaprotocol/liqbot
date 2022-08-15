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
	"code.vegaprotocol.io/liqbot/util"
)

// TODO: maybe after staking, deposit back the same amount as staked.
func (b *bot) runPositionManagement(ctx context.Context) {
	defer b.log.Warning("PositionManagement: Stopped")

	if err := b.prePositionManagement(ctx); err != nil {
		b.log.WithFields(log.Fields{"error": err.Error()}).Error("PositionManagement: Failed to initialize")
		return
	}

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

			// Only update liquidity and position if we are not in auction
			if !b.canPlaceOrders() {
				if err := b.ensureCommitmentAmount(ctx); err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to update commitment amount")
				}

				if err = b.placeAuctionOrders(ctx); err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Warning("PositionManagement: Failed to place auction orders")
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

func (b *bot) ensureCommitmentAmount(ctx context.Context) error {
	requiredCommitment, err := b.getRequiredCommitment()
	if err != nil {
		return fmt.Errorf("failed to get new commitment amount: %w", err)
	}

	if requiredCommitment.IsZero() {
		return nil
	}

	b.log.WithFields(
		log.Fields{
			"newCommitment": requiredCommitment.String(),
		},
	).Debug("PositionManagement: Supplied stake is less than target stake, increasing commitment amount...")

	if err = b.ensureBalance(ctx, requiredCommitment, "PositionManagement"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	buys, sells, _ := b.getShape()

	b.log.WithFields(
		log.Fields{
			"newCommitment": requiredCommitment.String(),
		},
	).Debug("PositionManagement: Sending new commitment amount...")

	if err = b.sendLiquidityProvisionAmendment(ctx, requiredCommitment, buys, sells); err != nil {
		return fmt.Errorf("failed to update commitment amount: %w", err)
	}

	return nil
}

func (b *bot) getRequiredCommitment() (*num.Uint, error) {
	suppliedStake := b.Market().SuppliedStake().Clone()
	targetStake := b.Market().TargetStake().Clone()

	b.log.WithFields(log.Fields{
		"suppliedStake": suppliedStake.String(),
		"targetStake":   targetStake.String(),
	}).Debug("PositionManagement: Checking for required commitment")

	if targetStake.IsZero() {
		var err error
		targetStake, err = util.ConvertUint256(b.config.StrategyDetails.CommitmentAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to convert commitment amount: %w", err)
		}
	}

	dx := suppliedStake.Int().Sub(targetStake.Int())

	if dx.IsPositive() {
		return num.Zero(), nil
	}

	return num.Zero().Add(targetStake, dx.Uint()), nil
}

func (b *bot) manageDirection(ctx context.Context, previousOpenVolume int64) (int64, error) {
	openVolume := b.Market().OpenVolume()
	buyShape, sellShape, shape := b.getShape()

	b.log.WithFields(log.Fields{
		"openVolume":         b.Market().OpenVolume(),
		"previousOpenVolume": previousOpenVolume,
		"shape":              shape,
	}).Debug("PositionManagement: Checking for direction change")

	// If we flipped then send the new LP order
	if !b.shouldAmend(openVolume, previousOpenVolume) {
		return openVolume, nil
	}

	b.log.WithFields(log.Fields{"shape": shape}).Debug("PositionManagement: Flipping LP direction")

	commitment, err := b.getRequiredCommitment()
	if err != nil {
		return openVolume, fmt.Errorf("failed to get required commitment amount: %w", err)
	}

	if err = b.ensureBalance(ctx, commitment, "PositionManagement"); err != nil {
		return openVolume, fmt.Errorf("failed to ensure balance: %w", err)
	}

	if err = b.sendLiquidityProvisionAmendment(ctx, commitment, buyShape, sellShape); err != nil {
		return openVolume, fmt.Errorf("failed to send liquidity provision amendment: %w", err)
	}

	return openVolume, nil
}

func (b *bot) shouldAmend(openVolume, previousOpenVolume int64) bool {
	return (openVolume > 0 && previousOpenVolume <= 0) || (b.Market().OpenVolume() < 0 && previousOpenVolume >= 0)
}

func (b *bot) managePosition(ctx context.Context) error {
	size, side, shouldPlace := b.checkPosition()
	b.log.WithFields(log.Fields{
		"currentPrice":   b.Market().MarkPrice().String(),
		"balanceGeneral": b.Balance().General().String(),
		"balanceMargin":  b.Balance().Margin().String(),
		"openVolume":     b.Market().OpenVolume(),
		"size":           size,
		"side":           side,
		"shouldPlace":    shouldPlace,
	}).Debug("PositionManagement: Checking for position management")

	if !shouldPlace {
		return nil
	}

	if err := b.submitOrder(
		ctx,
		size,
		num.Zero(),
		side,
		vega.Order_TIME_IN_FORCE_IOC,
		vega.Order_TYPE_MARKET,
		"PosManagement",
		"PositionManagement",
		0,
	); err != nil {
		return fmt.Errorf("failed to place order: %w", err)
	}

	return nil
}

func (b *bot) sendLiquidityProvision(ctx context.Context, commitment *num.Uint, buys, sells []*vega.LiquidityOrder) error {
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

	b.log.WithFields(log.Fields{
		"commitment":   commitment.String(),
		"balanceTotal": b.Balance().Total().String(),
	}).Debug("PositionManagement: Submitting LiquidityProvisionSubmission...")

	if err := b.walletClient.SignTx(ctx, submitTxReq); err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission: %w", err)
	}

	b.log.WithFields(log.Fields{
		"commitment":   commitment.String(),
		"balanceTotal": b.Balance().Total().String(),
	}).Debug("PositionManagement: Submitted LiquidityProvisionSubmission")
	return nil
}

// call this if the position flips.
func (b *bot) sendLiquidityProvisionAmendment(ctx context.Context, commitment *num.Uint, buys, sells []*vega.LiquidityOrder) error {
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
		"commitment": commitment.String(),
	}).Debug("PositionManagement: Submitted LiquidityProvisionAmendment")
	return nil
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
		"commitment":   "0",
		"balanceTotal": b.Balance().Total().String(),
	}).Debug("PositionManagement: Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *bot) checkPosition() (uint64, vega.Side, bool) {
	size := uint64(0)
	side := vega.Side_SIDE_UNSPECIFIED
	shouldPlace := true
	openVolume := b.Market().OpenVolume()

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

	commitment, err := b.getRequiredCommitment()
	if err != nil {
		return fmt.Errorf("failed to get required commitment: %w", err)
	}

	// TODO: is this ok?
	if commitment.IsZero() {
		return nil
	}

	if err = b.ensureBalance(ctx, commitment, "PositionManagement"); err != nil {
		return fmt.Errorf("failed to ensure balance: %w", err)
	}

	// Submit LP order to market.
	if err = b.sendLiquidityProvision(ctx, commitment, buyShape, sellShape); err != nil {
		return fmt.Errorf("failed to send liquidity provision order: %w", err)
	}

	return nil
}

func (b *bot) getShape() ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string) {
	// We always cache off with longening shapes
	shape := "longening"
	buyShape := b.config.StrategyDetails.LongeningShape.Buys.ToVegaLiquidityOrders()
	sellShape := b.config.StrategyDetails.LongeningShape.Sells.ToVegaLiquidityOrders()

	if b.Market().OpenVolume() > 0 {
		shape = "shortening"
		buyShape = b.config.StrategyDetails.ShorteningShape.Buys.ToVegaLiquidityOrders()
		sellShape = b.config.StrategyDetails.ShorteningShape.Sells.ToVegaLiquidityOrders()
	}

	return buyShape, sellShape, shape
}

func (b *bot) checkInitialMargin(buyShape, sellShape []*vega.LiquidityOrder) error {
	// Turn the shapes into a set of orders scaled by commitment
	obligation := b.Balance().Total()
	buyOrders := b.calculateOrderSizes(obligation, buyShape)
	sellOrders := b.calculateOrderSizes(obligation, sellShape)

	buyRisk := 0.01
	sellRisk := 0.01

	buyCost := b.calculateMarginCost(buyRisk, buyOrders)
	sellCost := b.calculateMarginCost(sellRisk, sellOrders)

	shapeMarginCost := num.Max(buyCost, sellCost)
	avail := mulFrac(b.Balance().General(), b.config.StrategyDetails.OrdersFraction, 15)

	if !avail.LT(shapeMarginCost) {
		return nil
	}

	missingPercent := "Inf"

	if !avail.IsZero() {
		x := num.UintChain(shapeMarginCost).Sub(avail).Mul(num.NewUint(100)).Div(avail).Get()
		missingPercent = fmt.Sprintf("%v%%", x)
	}

	b.log.WithFields(log.Fields{
		"available":      avail.String(),
		"cost":           shapeMarginCost.String(),
		"missing":        num.Zero().Sub(avail, shapeMarginCost).String(),
		"missingPercent": missingPercent,
	}).Error("PositionManagement: Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")

	return errors.New("not enough collateral")
}

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *bot) placeAuctionOrders(ctx context.Context) error {
	// Check if we have a currentPrice we can use
	if b.Market().MarkPrice().IsZero() {
		b.log.Debug("PositionManagement: No current price to place auction orders")
		return nil
	}

	b.log.WithFields(log.Fields{"currentPrice": b.Market().MarkPrice().String()}).Debug("PositionManagement: Placing auction orders")

	// Place the random orders split into
	totalVolume := num.Zero()

	rand.Seed(time.Now().UnixNano())

	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		time.Sleep(time.Second * 2)

		remaining := num.Zero().Sub(b.config.StrategyDetails.AuctionVolume.Get(), totalVolume)
		size := num.Min(num.UintChain(b.config.StrategyDetails.AuctionVolume.Get()).Div(num.NewUint(10)).Add(num.NewUint(1)).Get(), remaining)
		// #nosec G404
		price := num.Zero().Add(b.Market().MarkPrice(), num.NewUint(uint64(rand.Int63n(100)-50)))
		side := vega.Side_SIDE_BUY
		// #nosec G404
		if rand.Intn(2) == 0 {
			side = vega.Side_SIDE_SELL
		}

		tif := vega.Order_TIME_IN_FORCE_GTT
		orderType := vega.Order_TYPE_LIMIT
		ref := "AuctionOrder"

		if err := b.submitOrder(ctx, size.Uint64(), price, side, tif, orderType, ref, "PositionManagement", 330); err != nil {
			// We failed to send an order so stop trying to send anymore
			return fmt.Errorf("failed to send auction order: %w", err)
		}

		totalVolume = num.Zero().Add(totalVolume, size)
	}

	b.log.WithFields(log.Fields{"totalVolume": totalVolume.String()}).Debug("PositionManagement: Placed auction orders")

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
			Div(totalProportion).Div(b.Market().MarkPrice()).Get()
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

	totalMargin := num.UintChain(num.Zero()).Add(margins...).Mul(b.Market().MarkPrice()).Get()
	return mulFrac(totalMargin, risk, 15)
}

func mulFrac(n *num.Uint, x float64, precision float64) *num.Uint {
	val := num.NewUint(uint64(x * math.Pow(10, precision)))
	val.Mul(val, n)
	val.Div(val, num.NewUint(uint64(math.Pow(10, precision))))
	return val
}
