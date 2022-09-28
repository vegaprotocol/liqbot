package normal

import (
	"context"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	"code.vegaprotocol.io/protos/vega/wallet/v1"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

// TradingDataService implements the gRPC service of the same name.
type tradingDataService interface {
	MustDialConnection(ctx context.Context)
	Target() string
	Markets(req *dataapipb.MarketsRequest) (*dataapipb.MarketsResponse, error) // TODO: bot should probably not have to worry about finding markets
}

// TODO: PricingEngine response data could be cached in the data service, along with other external data sources.
// PricingEngine is the source of price information from the price proxy.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (ppservice.PriceResponse, error)
}

// TODO: move all account related stuff to an account service.
type WalletClient interface {
	CreateWallet(ctx context.Context, name, passphrase string) error
	LoginWallet(ctx context.Context, name, passphrase string) error
	ListPublicKeys(ctx context.Context) ([]string, error)
	GenerateKeyPair(ctx context.Context, passphrase string, meta []types.Meta) (*types.Key, error)
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) error
}

type dataStore interface {
	Balance() types.Balance // TODO: might not be needed by the bot.
	Market() types.MarketData
}

// TODO: this should be in some kind of a market service
type marketStream interface {
	Setup(walletPubKey string, pauseCh chan types.PauseSignal)
	WaitForStakeLinking() error
	WaitForProposalID() (string, error)
	WaitForProposalEnacted(pID string) error
}

// TODO: this could be improved: pubKey could be specified in config,
type dataStream interface {
	InitData(pubKey, marketID, settlementAssetID string, pauseCh chan types.PauseSignal) (data.GetDataStore, error)
}

// TODO: probably should be moved to be used by a new service (account/market/service?).
type whaleService interface {
	TopUp(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error
}

// MarketService should provide on-demand, up-to-date market data for the bot, as well as
// allow the bot to send liquidity provision, amendment and cancellation, and place orders.
type MarketService interface {
	Data() types.MarketData // TODO: include current external price (no caching because it keeps changing).
	ProvideLiquidity(ctx context.Context, buys, sells []*vega.LiquidityOrder) error
	FlipDirection(ctx context.Context, buys, sells []*vega.LiquidityOrder) error
	Order(ctx context.Context, price *num.Uint, size uint64, side vega.Side, tif vega.Order_TimeInForce, orderType vega.Order_Type, reference string) error
}
