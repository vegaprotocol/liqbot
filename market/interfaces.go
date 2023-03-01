package market

import (
	"context"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/market"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/vega/protos/vega"
)

type marketStream interface {
	Store() market.Store
	Subscribe(ctx context.Context, pubKey, marketID string) error
	WaitForProposalID() (string, error)
	WaitForProposalEnacted(pID string) error
	WaitForLiquidityProvision(ctx context.Context, ref string) error
	FindMarket(ctx context.Context, quote, base string) (*vega.Market, error)
}

type accountService interface {
	Balance(ctx context.Context, assetID string) cache.Balance
	EnsureBalance(ctx context.Context, asset *vega.Asset, balanceFn func(cache.Balance) *num.Uint, targetAmount *num.Uint, dp, scale uint64, from string) error
	EnsureStake(ctx context.Context, receiverName, receiverPubKey string, asset *vega.Asset, targetAmount *num.Uint, from string) error
}
