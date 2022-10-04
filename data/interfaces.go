package data

import (
	"context"

	"code.vegaprotocol.io/liqbot/types"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
)

// DataNode is a Vega Data node
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/market DataNode
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

type BalanceStore interface {
	Balance() types.Balance
	BalanceSet(sets ...func(*types.Balance))
}

type MarketStore interface {
	Market() types.MarketData
	OpenVolume() int64
	MarketSet(sets ...func(*types.MarketData))
}

type busEventer interface {
	processEvents(ctx context.Context, name string, req *vegaapipb.ObserveEventBusRequest, process func(*vegaapipb.ObserveEventBusResponse) (bool, error)) <-chan error
}
