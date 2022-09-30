package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.vegaprotocol.io/vega/core/events"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	"code.vegaprotocol.io/vega/protos/vega"

	log "github.com/sirupsen/logrus"

	coreapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"

	"code.vegaprotocol.io/liqbot/types"
)

type market struct {
	log          *log.Entry
	node         DataNode
	walletPubKey string
	marketID     string
	store        MarketStore
	busEvProc    busEventer
}

func NewMarketStream(node DataNode) *market {
	return &market{
		log:  log.WithField("module", "MarketStreamer"),
		node: node,
	}
}

func (m *market) Init(pubKey string, pauseCh chan types.PauseSignal) (MarketStore, error) {
	store := types.NewMarketStore()

	m.walletPubKey = pubKey
	m.store = store
	m.busEvProc = newBusEventProcessor(m.node, WithPauseCh(pauseCh))

	return store, nil
}

func (m *market) Subscribe(marketID string) error {
	m.marketID = marketID

	if err := m.initMarketData(); err != nil {
		return fmt.Errorf("failed to get market market: %w", err)
	}

	if err := m.initOpenVolume(); err != nil {
		return fmt.Errorf("failed to get open volume: %w", err)
	}

	go m.subscribeToMarketEvents()
	go m.subscribePositions()

	return nil
}

func (m *market) WaitForProposalID() (string, error) {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	var proposalID string

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.State == vega.Proposal_STATE_OPEN {
				proposalID = proposal.Id

				m.log.WithFields(log.Fields{
					"proposalID": proposalID,
				}).Info("Received proposal ID")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	m.busEvProc.processEvents(ctx, "Proposals", req, proc)
	return proposalID, ctx.Err()
}

func (m *market) WaitForProposalEnacted(pID string) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.State == vega.Proposal_STATE_ENACTED && proposal.Id == pID {
				m.log.WithFields(
					log.Fields{
						"proposalID": proposal.Id,
					}).Debug("Proposal was enacted")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	m.busEvProc.processEvents(ctx, "Proposals", req, proc)
	return ctx.Err()
}

func (m *market) subscribeToMarketEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: m.marketID,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			marketData := event.GetMarketData()

			md, err := types.FromVegaMD(marketData)
			if err != nil {
				return false, fmt.Errorf("failed to convert market market: %w", err)
			}

			m.store.MarketSet(types.SetMarketData(md))
		}
		return false, nil
	}

	m.busEvProc.processEvents(context.Background(), "MarketData", req, proc)
}

func (m *market) subscribePositions() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_SETTLE_POSITION,
		},
		PartyId:  m.walletPubKey,
		MarketId: m.marketID,
	}

	proc := func(ev *coreapipb.ObserveEventBusResponse) (bool, error) {
		ctx := context.Background()
		openVolume := m.store.OpenVolume()

		for _, event := range ev.Events {
			posEvt := events.SettlePositionEventFromStream(ctx, event)

			for _, p := range posEvt.Trades() {
				openVolume += p.Size()
			}
		}

		m.store.MarketSet(types.SetOpenVolume(openVolume))
		return false, nil
	}

	m.busEvProc.processEvents(context.Background(), "PositionData", req, proc)
}

func (m *market) initOpenVolume() error {
	positions, err := m.getPositions()
	if err != nil {
		return fmt.Errorf("failed to get position details: %w", err)
	}

	var openVolume int64
	// If we have not traded yet, then we won't have a position
	if positions != nil {
		if len(positions) != 1 {
			return errors.New("one position item required")
		}
		openVolume = positions[0].OpenVolume
	}

	m.store.MarketSet(types.SetOpenVolume(openVolume))
	return nil
}

// getPositions get this bot's positions.
func (m *market) getPositions() ([]*vega.Position, error) {
	response, err := m.node.PositionsByParty(&dataapipb.PositionsByPartyRequest{
		PartyId:  m.walletPubKey,
		MarketId: m.marketID,
	})
	if err != nil {
		return nil, err
	}

	return response.Positions, nil
}

// initMarketData gets the latest info about the market.
func (m *market) initMarketData() error {
	response, err := m.node.MarketDataByID(&dataapipb.MarketDataByIDRequest{MarketId: m.marketID})
	if err != nil {
		return fmt.Errorf("failed to get market market (ID:%s): %w", m.marketID, err)
	}

	md, err := types.FromVegaMD(response.MarketData)
	if err != nil {
		return fmt.Errorf("failed to convert market market: %w", err)
	}

	m.store.MarketSet(types.SetMarketData(md))
	return nil
}
