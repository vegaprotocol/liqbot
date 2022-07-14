package data

import (
	"fmt"
	"io"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"
)

type data struct {
	node              DataNode
	log               *log.Entry
	walletPubKey      string
	marketID          string
	settlementAssetID string
	store             dataStore
}

func NewStream(node DataNode, store dataStore) *data {
	return &data{
		node:  node,
		log:   log.WithField("module", "DataStreamer"),
		store: store,
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

	if err := s.subscribeToMarketEvents(); err != nil {
		return fmt.Errorf("failed to subscribeToMarketEvents: %w", err)
	}

	if err := s.subscribeToAccountEvents(); err != nil {
		return fmt.Errorf("failed to subscribeToAccountEvents: %w", err)
	}

	if err := s.subscribePositions(); err != nil {
		return fmt.Errorf("failed to subscribePositions: %w", err)
	}

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

func (s *data) subscribeToMarketEvents() error {
	req := &dataapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: s.marketID,
	}

	proc := func(event *eventspb.BusEvent) bool {
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

		return false
	}

	if err := s.processEvents(req, proc); err != nil {
		return fmt.Errorf("failed to process market events: %w", err)
	}

	return nil
}

// Party related events
func (s *data) subscribeToAccountEvents() error {
	req := &dataapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: s.walletPubKey,
	}

	proc := func(event *eventspb.BusEvent) bool {
		acct := event.GetAccount()
		// Filter out any that are for different assets
		if acct.Asset != s.settlementAssetID {
			return false
		}

		bal, errOrOverflow := num.UintFromString(acct.Balance, 10)
		if errOrOverflow {
			s.log.WithFields(log.Fields{
				"AccountBalance": acct.Balance,
			}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			return false
		}

		s.store.balanceSet(acct.Type, bal)

		return false
	}

	if err := s.processEvents(req, proc); err != nil {
		return fmt.Errorf("failed to process account events: %w", err)
	}

	return nil
}

func (s *data) subscribePositions() error {
	stream, err := s.node.PositionsSubscribe(&dataapipb.PositionsSubscribeRequest{
		MarketId: s.marketID,
		PartyId:  s.walletPubKey,
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to positions: %w", err)
	}

	go func() {
		for {
			o, err := stream.Recv()
			if err == io.EOF {
				log.Debugln("positions: stream closed by server err:", err)
				break
			}
			if err != nil {
				log.Debugln("positions: stream closed err:", err)
				break
			}

			s.store.openVolumeSet(o.GetPosition().OpenVolume)
		}
		// Let the app know we have stopped receiving position updates
		// s.positionStreamLive = false TODO
	}()

	return nil
}

func (s *data) processEvents(request *dataapipb.ObserveEventBusRequest, cb func(event *eventspb.BusEvent) bool) error {
	// First we have to create the stream
	stream, err := s.node.ObserveEventBus()
	if err != nil {
		return fmt.Errorf("failed to subscribe to event bus data: %w", err)
	}

	// Then we subscribe to the data
	if err = stream.SendMsg(request); err != nil {
		return fmt.Errorf("unable to send event bus request on the stream: %w", err)
	}

	go func() {
		for {
			eb, err := stream.Recv()
			if err == io.EOF {
				s.log.Warning("event bus data: stream closed by server (EOF)")
				break
			}
			if err != nil {
				s.log.WithFields(log.Fields{
					"error": err,
				}).Warning("event bus data: stream closed")
				break
			}

			for _, event := range eb.Events {
				if cb(event) {
					return
				}
			}
		}
	}()

	return nil
}
