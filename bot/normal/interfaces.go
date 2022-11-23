package normal

import (
	"context"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/shared/libs/types"
	wtypes "code.vegaprotocol.io/shared/libs/wallet/types"
	"code.vegaprotocol.io/vega/protos/vega"
	"code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

// TODO: move all account related stuff to an account service.
type WalletClient interface {
	CreateWallet(ctx context.Context, name, passphrase string) (string, error)
	LoginWallet(ctx context.Context, name, passphrase string) error
	ListPublicKeys(ctx context.Context) ([]string, error)
	GenerateKeyPair(ctx context.Context, passphrase string, meta []wtypes.Meta) (*wtypes.Key, error)
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) error
}

/*
// MarketService should provide on-demand, up-to-date market data for the bot, as well as
// allow the bot to send liquidity provision, amendment and cancellation, and place orders.

	type MarketService interface {
		Data() types.MarketData // TODO: include current external price (no caching because it keeps changing).
		ProvideLiquidity(ctx context.Context, buys, sells []*vega.LiquidityOrder) error
		FlipDirection(ctx context.Context, buys, sells []*vega.LiquidityOrder) error
		Order(ctx context.Context, price *num.Uint, size uint64, side vega.Side, tif vega.Order_TimeInForce, orderType vega.Order_Type, reference string) error
	}
*/
type marketService interface {
	Init(pubKey string, pauseCh chan types.PauseSignal) error
	Start(ctx context.Context, marketID string) error
	Market() cache.MarketData
	CanPlaceOrders() bool
	SubmitOrder(ctx context.Context, order *vega.Order, from string, secondsFromNow int64) error
	SeedOrders(ctx context.Context, from string) error
	SetupMarket(ctx context.Context) (*vega.Market, error)
	GetExternalPrice() (*num.Uint, error)
}

type accountService interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	Balance(ctx context.Context) cache.Balance
	EnsureBalance(ctx context.Context, assetID string, balanceFn func(cache.Balance) *num.Uint, targetAmount *num.Uint, from string) error
}
