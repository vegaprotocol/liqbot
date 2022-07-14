package data

import (
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

// DataNode is a Vega Data node
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/data DataNode
type DataNode interface {
	ObserveEventBus() (client vegaapipb.CoreService_ObserveEventBusClient, err error)
	PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error)
	MarketDataByID(req *dataapipb.MarketDataByIDRequest) (response *dataapipb.MarketDataByIDResponse, err error)
	PositionsByParty(req *dataapipb.PositionsByPartyRequest) (response *dataapipb.PositionsByPartyResponse, err error)
	PositionsSubscribe(req *dataapipb.PositionsSubscribeRequest) (client dataapipb.TradingDataService_PositionsSubscribeClient, err error)
}

type dataStore interface {
	balanceSet(typ vega.AccountType, bal *num.Uint)
	marketDataSet(marketData *types.MarketData)
	openVolumeSet(openVolume int64)
	cache()
}
