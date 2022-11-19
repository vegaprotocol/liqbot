package data

import (
	"context"

	"code.vegaprotocol.io/liqbot/types"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	vegaapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
)

// DataNode is a Vega Data node
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/data DataNode
type DataNode interface {
	busStreamer
	PartyAccounts(ctx context.Context, req *dataapipb.ListAccountsRequest) ([]*dataapipb.AccountBalance, error)
	PartyStake(ctx context.Context, req *dataapipb.GetStakeRequest) (response *dataapipb.GetStakeResponse, err error)
	MarketDataByID(ctx context.Context, req *dataapipb.GetLatestMarketDataRequest) (*vega.MarketData, error)
	PositionsByParty(ctx context.Context, req *dataapipb.ListPositionsRequest) ([]*vega.Position, error)
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
