package data

import (
	"code.vegaprotocol.io/protos/vega"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

type store struct {
	balanceGetCh chan balanceGetReq
	balanceSetCh chan balanceSetReq

	marketDataGetCh chan marketDataGetReq
	marketDataSetCh chan marketDataSetReq

	openVolumeGetCh chan openVolumeGetReq
	openVolumeSetCh chan openVolumeSetReq
}

func NewStore() *store {
	return &store{
		balanceGetCh:    make(chan balanceGetReq),
		balanceSetCh:    make(chan balanceSetReq),
		marketDataGetCh: make(chan marketDataGetReq),
		marketDataSetCh: make(chan marketDataSetReq),
		openVolumeGetCh: make(chan openVolumeGetReq),
		openVolumeSetCh: make(chan openVolumeSetReq),
	}
}

type balanceGetReq struct {
	resp chan *types.Balance
}

type balanceSetReq struct {
	typ     vega.AccountType
	balance *num.Uint
}

type marketDataGetReq struct {
	resp chan *types.MarketData
}

type marketDataSetReq struct {
	marketData *types.MarketData
}

type openVolumeGetReq struct {
	resp chan int64
}

type openVolumeSetReq struct {
	openVolume int64
}

func (s *store) BalanceGet() *types.Balance {
	resp := make(chan *types.Balance)
	s.balanceGetCh <- balanceGetReq{resp}
	return <-resp
}

func (s *store) balanceSet(typ vega.AccountType, bal *num.Uint) {
	s.balanceSetCh <- balanceSetReq{typ: typ, balance: bal.Clone()}
}

func (s *store) MarketDataGet() *types.MarketData {
	resp := make(chan *types.MarketData)
	s.marketDataGetCh <- marketDataGetReq{resp}
	return <-resp
}

func (s *store) marketDataSet(marketData *types.MarketData) {
	s.marketDataSetCh <- marketDataSetReq{marketData}
}

func (s *store) OpenVolumeGet() int64 {
	resp := make(chan int64)
	s.openVolumeGetCh <- openVolumeGetReq{resp}
	return <-resp
}

func (s *store) openVolumeSet(openVolume int64) {
	s.openVolumeSetCh <- openVolumeSetReq{openVolume}
}

func (s *store) cache() {
	d := struct {
		balance     *types.Balance
		marketData  *types.MarketData
		tradingMode *vega.Market_TradingMode
		openVolume  int64
	}{
		balance:    new(types.Balance),
		marketData: new(types.MarketData),
	}

	for {
		select {
		case req := <-s.balanceGetCh:
			req.resp <- d.balance
		case req := <-s.balanceSetCh:
			switch req.typ {
			case vega.AccountType_ACCOUNT_TYPE_GENERAL:
				d.balance.General = req.balance
			case vega.AccountType_ACCOUNT_TYPE_MARGIN:
				d.balance.Margin = req.balance
			case vega.AccountType_ACCOUNT_TYPE_BOND:
				d.balance.Bond = req.balance
			}
		case req := <-s.marketDataGetCh:
			if d.marketData != nil {
				mdVal := *d.marketData
				req.resp <- &mdVal
			}
		case req := <-s.marketDataSetCh:
			d.marketData = req.marketData
		case req := <-s.openVolumeGetCh:
			req.resp <- d.openVolume
		case req := <-s.openVolumeSetCh:
			d.openVolume = req.openVolume
		default:
			// Do nothing
		}
	}
}
