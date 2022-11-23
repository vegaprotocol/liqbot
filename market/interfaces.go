package market

import (
	"context"

	"code.vegaprotocol.io/shared/libs/cache"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/shared/libs/types"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	vegaapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
)

// TODO: this could be improved: pubKey could be specified in config.
type marketStream interface {
	Init(pubKey string, pauseCh chan types.PauseSignal) (marketStore, error)
	Subscribe(ctx context.Context, marketID string) error
	waitForProposalID() (string, error)
	waitForProposalEnacted(pID string) error
}

type dataNode interface {
	MarketDataByID(ctx context.Context, req *dataapipb.GetLatestMarketDataRequest) (*vega.MarketData, error)
	PositionsByParty(ctx context.Context, req *dataapipb.ListPositionsRequest) ([]*vega.Position, error)
	ObserveEventBus(ctx context.Context) (client vegaapipb.CoreService_ObserveEventBusClient, err error)
	MustDialConnection(ctx context.Context)
	Target() string
	Markets(ctx context.Context, req *dataapipb.ListMarketsRequest) ([]*vega.Market, error) // TODO: bot should probably not have to worry about finding markets
}

type accountService interface {
	EnsureBalance(ctx context.Context, assetID string, balanceFn func(cache.Balance) *num.Uint, targetAmount *num.Uint, from string) error
	EnsureStake(ctx context.Context, receiverName, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error
}

type marketStore interface {
	Market() cache.MarketData
	OpenVolume() int64
	MarketSet(sets ...func(*cache.MarketData))
}

type busEventer interface {
	ProcessEvents(ctx context.Context, name string, req *vegaapipb.ObserveEventBusRequest, process func(*vegaapipb.ObserveEventBusResponse) (bool, error)) <-chan error
}
