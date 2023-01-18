package normal

import (
	"context"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/vega/protos/vega"
)

type marketService interface {
	Market() cache.MarketData
	CanPlaceOrders() bool
	SubmitOrder(ctx context.Context, order *vega.Order, from string, secondsFromNow int64) error
	SetupMarket(ctx context.Context) (*vega.Market, error)
	EnsureCommitmentAmount(ctx context.Context) error
	GetExternalPrice() (*num.Uint, error)
	GetShape() ([]*vega.LiquidityOrder, []*vega.LiquidityOrder, string)
	CheckPosition() (uint64, vega.Side, bool)
	SendLiquidityProvisionAmendment(ctx context.Context, commitment *num.Uint, buys, sells []*vega.LiquidityOrder) error
	SeedOrders(ctx context.Context) error
}

type accountService interface {
	Balance(ctx context.Context, assetID string) cache.Balance
	EnsureBalance(ctx context.Context, assetID string, balanceFn func(cache.Balance) *num.Uint, targetAmount *num.Uint, dp, scale uint64, from string) error
}
