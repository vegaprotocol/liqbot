package data

import (
	"context"
	"fmt"

	"code.vegaprotocol.io/vega/events"

	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
)

type data struct {
	log               *log.Entry
	node              DataNode
	walletPubKey      string
	marketID          string
	settlementAssetID string
	store             setDataStore
	busEvProc         busEventer
}

func NewStreamData(
	node DataNode,
) *data {
	return &data{
		log:  log.WithField("module", "DataStreamer"),
		node: node,
	}
}

func (d *data) InitData(
	pubKey,
	marketID,
	settlementAssetID string,
	pauseCh chan types.PauseSignal,
) (GetDataStore, error) {
	store := types.NewStore()

	d.walletPubKey = pubKey
	d.marketID = marketID
	d.settlementAssetID = settlementAssetID
	d.store = store
	d.busEvProc = newBusEventProcessor(d.node, pauseCh)

	if err := d.setInitialData(); err != nil {
		return nil, fmt.Errorf("failed to set initial data: %w", err)
	}

	go d.subscribeToMarketEvents()
	go d.subscribeToAccountEvents()
	go d.subscribePositions()

	return store, nil
}

func (d *data) setInitialData() error {
	if err := d.initBalance(); err != nil {
		return fmt.Errorf("failed to set account balance: %w", err)
	}

	if err := d.initMarketData(); err != nil {
		return fmt.Errorf("failed to get market data: %w", err)
	}

	if err := d.initOpenVolume(); err != nil {
		return fmt.Errorf("failed to get open volume: %w", err)
	}

	return nil
}

// TODO: should be used by a new market service instead of by the bot
func (d *data) subscribeToMarketEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: d.marketID,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			marketData := event.GetMarketData()

			md, err := types.FromVegaMD(marketData)
			if err != nil {
				return false, fmt.Errorf("failed to convert market data: %w", err)
			}

			d.store.MarketSet(types.SetMarketData(md))
		}
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "MarketData", req, proc)
}

// TODO: should be used by a new account service instead of by the bot
// Party related events.
func (d *data) subscribeToAccountEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: d.walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			acct := event.GetAccount()
			// filter out any that are for different assets
			if acct.Asset != d.settlementAssetID {
				continue
			}

			if err := d.setBalanceByType(acct); err != nil {
				d.log.WithFields(
					log.Fields{
						"error":       err.Error(),
						"accountType": acct.Type.String(),
					},
				).Error("failed to set account balance")
			}
		}
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "AccountData", req, proc)
}

// TODO: should be used by a new market service instead of by the bot
func (d *data) subscribePositions() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_SETTLE_POSITION,
		},
		PartyId:  d.walletPubKey,
		MarketId: d.marketID,
	}

	proc := func(ev *coreapipb.ObserveEventBusResponse) (bool, error) {
		ctx := context.Background()
		openVolume := d.store.OpenVolume()

		for _, event := range ev.Events {
			posEvt := events.SettlePositionEventFromStream(ctx, event)

			for _, p := range posEvt.Trades() {
				openVolume += p.Size()
			}
		}

		d.store.MarketSet(types.SetOpenVolume(openVolume))
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "PositionData", req, proc)
}
