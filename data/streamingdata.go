package data

import (
	"fmt"

	v1 "code.vegaprotocol.io/protos/data-node/api/v1"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"

	"code.vegaprotocol.io/protos/vega"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"
)

type data struct {
	log               *log.Entry
	node              DataNode
	walletPubKey      string
	marketID          string
	settlementAssetID string
	store             dataStore
	busEvProc         *eventProcessor[*coreapipb.ObserveEventBusRequest, *coreapipb.ObserveEventBusResponse]
	posEvProc         *eventProcessor[*v1.PositionsSubscribeRequest, *v1.PositionsSubscribeResponse]
}

func NewStream(node DataNode, store dataStore, pauseCh chan types.PauseSignal) *data {
	return &data{
		log:       log.WithField("module", "DataStreamer"),
		node:      node,
		store:     store,
		busEvProc: newBusEventProcessor(node, pauseCh),
		posEvProc: newPosEventProcessor(node, pauseCh),
	}
}

func (s *data) InitForData(marketID, pubKey, settlementAssetID string) error {
	go s.store.cache()

	s.walletPubKey = pubKey
	s.marketID = marketID
	s.settlementAssetID = settlementAssetID

	if err := s.setInitialData(); err != nil {
		return fmt.Errorf("failed to set initial data: %w", err)
	}

	s.subscribeToMarketEvents()
	s.subscribeToAccountEvents()
	s.subscribePositions()
	return nil
}

func (s *data) setInitialData() error {
	// Collateral
	balanceGeneral, err := s.getAccountGeneral()
	if err != nil {
		return fmt.Errorf("failed to get account general: %w", err)
	}

	s.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_GENERAL, balanceGeneral)

	balanceMargin, err := s.getAccountMargin()
	if err != nil {
		return fmt.Errorf("failed to get account margin: %w", err)
	}

	s.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_MARGIN, balanceMargin)

	balanceBond, err := s.getAccountBond()
	if err != nil {
		return fmt.Errorf("failed to get account bond: %w", err)
	}

	s.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_BOND, balanceBond)

	marketData, err := s.getMarketData()
	if err != nil {
		return fmt.Errorf("failed to get market data: %w", err)
	}

	currentPrice, err := convertUint256(marketData.StaticMidPrice)
	if err != nil {
		return fmt.Errorf("failed to convert current price: %w", err)
	}

	s.store.marketDataSet(&types.MarketData{
		StaticMidPrice: currentPrice,
		MarkPrice:      &num.Uint{},
		TradingMode:    marketData.MarketTradingMode,
	})

	openVolume, err := s.getOpenVolume()
	if err != nil {
		return fmt.Errorf("failed to get open volume: %w", err)
	}

	s.store.openVolumeSet(openVolume)

	return nil
}

func (s *data) subscribeToMarketEvents() {
	proc := func(rsp *coreapipb.ObserveEventBusResponse) error {
		for _, event := range rsp.Events {
			marketData := event.GetMarketData()

			markPrice, errOrOverflow := num.UintFromString(marketData.MarkPrice, 10)
			if errOrOverflow {
				s.log.WithFields(log.Fields{
					"markPrice": marketData.MarkPrice,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			staticMidPrice, err := convertUint256(marketData.StaticMidPrice)
			if err != nil {
				s.log.WithFields(log.Fields{
					"staticMidPrice": marketData.StaticMidPrice,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			s.store.marketDataSet(&types.MarketData{
				StaticMidPrice: staticMidPrice,
				MarkPrice:      markPrice,
				TradingMode:    marketData.MarketTradingMode,
			})

			return nil
		}
		return nil
	}

	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: s.marketID,
	}

	go s.busEvProc.processEvents("MarketData", req, proc)
}

// Party related events
func (s *data) subscribeToAccountEvents() {
	proc := func(rsp *coreapipb.ObserveEventBusResponse) error {
		for _, event := range rsp.Events {
			acct := event.GetAccount()
			// filter out any that are for different assets
			if acct.Asset != s.settlementAssetID {
				continue
			}

			bal, errOrOverflow := num.UintFromString(acct.Balance, 10)
			if errOrOverflow {
				s.log.WithFields(log.Fields{
					"AccountBalance": acct.Balance,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
				continue
			}

			s.store.balanceSet(acct.Type, bal)
		}
		return nil
	}

	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: s.walletPubKey,
	}

	go s.busEvProc.processEvents("AccountData", req, proc)
}

func (s *data) subscribePositions() {
	req := &v1.PositionsSubscribeRequest{
		MarketId: s.marketID,
		PartyId:  s.walletPubKey,
	}

	proc := func(ev *v1.PositionsSubscribeResponse) error {
		s.store.openVolumeSet(ev.GetPosition().OpenVolume)
		return nil
	}

	go s.posEvProc.processEvents("PositionData", req, proc)
}
