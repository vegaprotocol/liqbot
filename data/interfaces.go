package data

import (
	"context"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

// DataNode is a Vega Data node
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/data DataNode
type DataNode interface {
	ObserveEventBus(ctx context.Context) (client vegaapipb.CoreService_ObserveEventBusClient, err error)
	PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error)
	MarketDataByID(req *dataapipb.MarketDataByIDRequest) (response *dataapipb.MarketDataByIDResponse, err error)
	PositionsByParty(req *dataapipb.PositionsByPartyRequest) (response *dataapipb.PositionsByPartyResponse, err error)
	MustDialConnection(ctx context.Context)
}

type dataStore interface {
	balanceSet(typ vega.AccountType, bal *num.Uint)
	balanceInit(balance *types.Balance)
	marketDataSet(marketData *types.MarketData)
	openVolumeSet(openVolume int64)
	cache()
	OpenVolume() int64
}

type busEventer interface {
	processEvents(ctx context.Context, name string, req *vegaapipb.ObserveEventBusRequest, process func(*vegaapipb.ObserveEventBusResponse) (bool, error))
}
