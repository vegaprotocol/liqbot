package market

import (
	"context"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
)

// TODO: PricingEngine response data could be cached in the data service, along with other external data sources.
// PricingEngine is the source of price information from the price proxy.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (ppservice.PriceResponse, error)
}

// TODO: this could be improved: pubKey could be specified in config,
type marketStream interface {
	Init(pubKey string, pauseCh chan types.PauseSignal) (data.MarketStore, error)
	Subscribe(marketID string) error
	WaitForProposalID() (string, error)
	WaitForProposalEnacted(pID string) error
}

type tradingDataService interface {
	MustDialConnection(ctx context.Context)
	Target() string
	Markets(req *dataapipb.MarketsRequest) (*dataapipb.MarketsResponse, error) // TODO: bot should probably not have to worry about finding markets
}

type accountService interface {
	EnsureBalance(ctx context.Context, assetID string, targetAmount *num.Uint, from string) error
	EnsureStake(ctx context.Context, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error
}
