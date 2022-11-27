package market

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/shared/libs/cache"
	sevents "code.vegaprotocol.io/shared/libs/events"
	"code.vegaprotocol.io/shared/libs/types"
	"code.vegaprotocol.io/vega/core/events"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	coreapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
)

type market struct {
	name         string
	log          *log.Entry
	node         dataNode
	walletPubKey string
	marketID     string
	store        marketStore
	busEvProc    busEventer
}

func NewMarketStream(name string, node dataNode) *market {
	return &market{
		name: name,
		node: node,
		log:  log.WithField("component", "MarketStreamer"),
	}
}

func (m *market) Init(pubKey string, pauseCh chan types.PauseSignal) (marketStore, error) {
	store := cache.NewMarketStore()

	m.walletPubKey = pubKey
	m.store = store
	m.busEvProc = sevents.NewBusEventProcessor(m.node, sevents.WithPauseCh(pauseCh))

	return store, nil
}

func (m *market) Subscribe(ctx context.Context, marketID string) error {
	m.marketID = marketID

	if err := m.initMarketData(ctx); err != nil {
		return fmt.Errorf("failed to get market market: %w", err)
	}

	if err := m.initOpenVolume(ctx); err != nil {
		return fmt.Errorf("failed to get open volume: %w", err)
	}

	m.subscribeToMarketEvents()
	m.subscribePositions()
	m.subscribeToOrderEvents()

	return nil
}

func (m *market) waitForProposalID() (string, error) {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	var proposalID string

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.PartyId != m.walletPubKey {
				continue
			}

			if proposal.State != vega.Proposal_STATE_OPEN {
				return true, fmt.Errorf("failed to propose market: %s; code: %s",
					proposal.ErrorDetails, proposal.State.String())
			}

			proposalID = proposal.Id

			m.log.WithFields(log.Fields{
				"proposalID": proposalID,
			}).Info("Received proposal ID")
			return true, nil
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*450)
	defer cancel()

	errCh := m.busEvProc.ProcessEvents(ctx, "Proposals: "+m.name, req, proc)
	select {
	case err := <-errCh:
		return proposalID, err
	case <-ctx.Done():
		return "", fmt.Errorf("timed out waiting for proposal ID")
	}
}

func (m *market) waitForProposalEnacted(pID string) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.Id != pID {
				continue
			}

			if proposal.State != vega.Proposal_STATE_ENACTED {
				if proposal.State == vega.Proposal_STATE_OPEN {
					continue
				}
			} else {
				return true, fmt.Errorf("failed to enact market: %s; code: %s",
					proposal.ErrorDetails, proposal.State.String())
			}

			m.log.WithFields(
				log.Fields{
					"proposalID": proposal.Id,
				}).Debug("Proposal was enacted")
			return true, nil
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*450)
	defer cancel()

	errCh := m.busEvProc.ProcessEvents(ctx, "Proposals: "+m.name, req, proc)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for proposal enactment")
	}
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

			md, err := cache.FromVegaMD(marketData)
			if err != nil {
				return false, fmt.Errorf("failed to convert market market: %w", err)
			}

			m.store.MarketSet(cache.SetMarketData(md))
		}
		return false, nil
	}

	m.busEvProc.ProcessEvents(context.Background(), "MarketData: "+m.name, req, proc)
}

func (m *market) subscribeToOrderEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ORDER,
		},
		PartyId:  m.walletPubKey,
		MarketId: m.marketID,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			order := event.GetOrder()

			if order.Status == vega.Order_STATUS_REJECTED {
				m.log.WithFields(log.Fields{
					"orderID":        order.Id,
					"order.status":   order.Status.String(),
					"order.PartyId":  order.PartyId,
					"order.marketID": order.MarketId,
					"reason":         order.Reason,
				}).Warn("Order was rejected")
			}
		}
		return false, nil
	}

	m.busEvProc.ProcessEvents(context.Background(), "Order: "+m.name, req, proc)
}

// balance.general: 4389751733879200
// order.price: 543283
// order.size: 400
// "OrderError: Margin Check Failed"

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

		m.store.MarketSet(cache.SetOpenVolume(openVolume))
		return false, nil
	}

	m.busEvProc.ProcessEvents(context.Background(), "PositionData: "+m.name, req, proc)
}

func (m *market) initOpenVolume(ctx context.Context) error {
	positions, err := m.getPositions(ctx)
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

	m.store.MarketSet(cache.SetOpenVolume(openVolume))
	return nil
}

// getPositions get this bot's positions.
func (m *market) getPositions(ctx context.Context) ([]*vega.Position, error) {
	response, err := m.node.PositionsByParty(ctx, &dataapipb.ListPositionsRequest{
		PartyId:  m.walletPubKey,
		MarketId: m.marketID,
	})
	if err != nil {
		return nil, err
	}

	return response, nil
}

// initMarketData gets the latest info about the market.
func (m *market) initMarketData(ctx context.Context) error {
	response, err := m.node.MarketDataByID(ctx, &dataapipb.GetLatestMarketDataRequest{MarketId: m.marketID})
	if err != nil {
		return fmt.Errorf("failed to get market market (ID:%s): %w", m.marketID, err)
	}

	md, err := cache.FromVegaMD(response)
	if err != nil {
		return fmt.Errorf("failed to convert market market: %w", err)
	}

	m.store.MarketSet(cache.SetMarketData(md))
	return nil
}
