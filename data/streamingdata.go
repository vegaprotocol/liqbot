package data

import (
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "code.vegaprotocol.io/protos/vega/api/v1"

	e "code.vegaprotocol.io/liqbot/errors"
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
	pause             chan struct{}
	botPaused         bool
	mu                sync.Mutex
}

func NewStream(node DataNode, store dataStore, pause chan struct{}) *data {
	return &data{
		node:  node,
		log:   log.WithField("module", "DataStreamer"),
		store: store,
		pause: pause,
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
	getStream := func() (dataapipb.TradingDataService_PositionsSubscribeClient, error) {
		return s.node.PositionsSubscribe(&dataapipb.PositionsSubscribeRequest{
			MarketId: s.marketID,
			PartyId:  s.walletPubKey,
		})
	}
	stream := mustConnectStream[dataapipb.TradingDataService_PositionsSubscribeClient](s.log, s.node, getStream)

	go func() {
		for {
			o, err := stream.Recv()
			if err != nil {
				s.log.WithFields(log.Fields{
					"error": err,
				}).Warning("positions: stream closed, resubscribing")

				s.pauseBot(true)

				stream = mustConnectStream[dataapipb.TradingDataService_PositionsSubscribeClient](s.log, s.node, getStream)
				s.pauseBot(false)
				continue
			}

			s.store.openVolumeSet(o.GetPosition().OpenVolume)
		}
	}()

	return nil
}

func (s *data) processEvents(request *dataapipb.ObserveEventBusRequest, cb func(event *eventspb.BusEvent) bool) error {
	stream, err := s.getBusStream(request)
	if err != nil {
		return err
	}

	go func() {
		for {
			eb, err := stream.Recv()
			if err != nil {
				s.log.WithFields(log.Fields{
					"error": err,
				}).Warning("event bus data: stream closed, resubscribing")

				s.pauseBot(true)

				if stream, err = s.getBusStream(request); err != nil {
					s.log.WithFields(log.Fields{
						"error": err,
					}).Warning("unable to send event bus request on the stream: %w")
					continue
				}
				s.pauseBot(false)
				continue
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

func (s *data) pauseBot(p bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p && !s.botPaused {
		s.log.Info("Pausing bot")
		s.botPaused = true
	} else if !p && s.botPaused {
		s.log.Info("Resuming bot")
		s.botPaused = false
	} else {
		return
	}
	go func() {
		s.pause <- struct{}{}
	}()
}

func (s *data) getBusStream(request *dataapipb.ObserveEventBusRequest) (v1.CoreService_ObserveEventBusClient, error) {
	getStream := func() (v1.CoreService_ObserveEventBusClient, error) {
		stream, err := s.node.ObserveEventBus()
		if err != nil {
			return nil, err
		}
		// Then we subscribe to the data
		if err := stream.SendMsg(request); err != nil {
			s.log.WithFields(log.Fields{
				"error": err,
			}).Errorf("unable to send event bus request on the stream")
			return nil, err
		}
		return stream, nil
	}
	//always get a new stream
	return mustConnectStream[v1.CoreService_ObserveEventBusClient](s.log, s.node, getStream), nil
}

func mustConnectStream[stream any](l *log.Entry, n DataNode, getStream func() (stream, error)) stream {
	var (
		s   stream
		err error
	)

	attempt := 0
	sleepTime := time.Second * 3

	for s, err = getStream(); err != nil; s, err = getStream() {
		attempt++

		l.WithFields(log.Fields{
			"error":   err,
			"attempt": attempt,
		}).Errorf("event bus data: failed to subscribe to stream, retrying in %s...", sleepTime)

		if errors.Unwrap(err).Error() == e.ErrConnectionNotReady.Error() {
			l.Debug("attempting to reconnect to gRPC server")
			if err = n.DialConnection(); err != nil {
				l.WithFields(log.Fields{
					"error": err,
				}).Error("event bus data: failed to dial connection")
			}
		}

		time.Sleep(sleepTime)
	}

	return s
}
