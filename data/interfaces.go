package data

import (
	"context"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"

	"code.vegaprotocol.io/liqbot/types"
)

// DataNode is a Vega Data node
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/data DataNode
type DataNode interface {
	busStreamer
	PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error)
	MarketDataByID(req *dataapipb.MarketDataByIDRequest) (response *dataapipb.MarketDataByIDResponse, err error)
	PositionsByParty(req *dataapipb.PositionsByPartyRequest) (response *dataapipb.PositionsByPartyResponse, err error)
}

type busStreamer interface {
	MustDialConnection(ctx context.Context)
	ObserveEventBus(ctx context.Context) (client vegaapipb.CoreService_ObserveEventBusClient, err error)
}

type GetDataStore interface {
	Balance() types.Balance
	Market() types.MarketData
}

type setDataStore interface {
	BalanceSet(sets ...func(*types.Balance))
	OpenVolume() int64
	MarketSet(sets ...func(*types.MarketData))
}

type busEventer interface {
	processEvents(ctx context.Context, name string, req *vegaapipb.ObserveEventBusRequest, process func(*vegaapipb.ObserveEventBusResponse) (bool, error))
}
