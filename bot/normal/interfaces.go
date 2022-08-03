package normal

import (
	"context"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	"code.vegaprotocol.io/protos/vega/wallet/v1"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

// TradingDataService implements the gRPC service of the same name.
type tradingDataService interface {
	Target() string
	Markets(req *dataapipb.MarketsRequest) (*dataapipb.MarketsResponse, error)       // BOT
	AssetByID(req *dataapipb.AssetByIDRequest) (*dataapipb.AssetByIDResponse, error) // BOT
}

// PricingEngine is the source of price information from the price proxy.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (ppservice.PriceResponse, error)
}

type WalletClient interface {
	CreateWallet(ctx context.Context, name, passphrase string) error
	LoginWallet(ctx context.Context, name, passphrase string) error
	ListPublicKeys(ctx context.Context) ([]string, error)
	GenerateKeyPair(ctx context.Context, passphrase string, meta []types.Meta) (*types.Key, error)
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) error
}

type dataStore interface {
	Balance() types.Balance
	TradingMode() vega.Market_TradingMode
	StaticMidPrice() *num.Uint
	TargetStake() *num.Uint
	SuppliedStake() *num.Uint
	MarkPrice() *num.Uint
	OpenVolume() int64
}

type marketStream interface {
	WaitForStakeLinking() error
	WaitForProposalID() (string, error)
	WaitForProposalEnacted(pID string) error
}

type dataStream interface {
	WaitForDepositFinalize(amount *num.Uint) error
}

type tokenService interface {
	Stake(ctx context.Context, amount *num.Uint) error
	Deposit(ctx context.Context, amount *num.Uint) error
}
