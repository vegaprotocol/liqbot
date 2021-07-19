package normal

import (
	"io"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	eventspb "github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/events/v1"
)

// Subscribe to all the events that we need to keep the bot happy
// These include:
// * Account Information (Margin, General and Bond)
// * Market Data
func (b *Bot) subscribeToEvents() error {
	// Party related events
	eventBusDataReq := &api.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: b.walletPubKey,
	}
	// First we have to create the stream
	stream, err := b.node.ObserveEventBus()
	if err != nil {
		return errors.Wrap(err, "Failed to subscribe to event bus data")
	}

	// Then we subscribe to the data
	err = stream.SendMsg(eventBusDataReq)
	if err != nil {
		return errors.Wrap(err, "Unable to send event bus request on the stream")
	}
	go b.processEventBusData(stream)

	// Market related events
	eventBusDataReq2 := &api.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: b.market.Id,
	}
	stream2, err := b.node.ObserveEventBus()
	if err != nil {
		return errors.Wrap(err, "Failed to subscribe to event bus data: ")
	}

	// Then we subscribe to the data
	err = stream2.SendMsg(eventBusDataReq2)
	if err != nil {
		return errors.Wrap(err, "Unable to send event bus request on the stream")
	}
	b.eventStreamLive = true
	go b.processEventBusData(stream2)
	return nil
}

func (b *Bot) processEventBusData(stream api.TradingDataService_ObserveEventBusClient) {
	for {
		eb, err := stream.Recv()
		if err == io.EOF {
			log.Warning("event bus data: stream closed by server err:", err)
			break
		}
		if err != nil {
			log.Warning("event bus data: stream closed err:", err)
			break
		}

		for _, event := range eb.Events {
			switch event.Type {
			case eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT:
				acct := event.GetAccount()
				// Filter out any that are for different assets
				if acct.Asset != b.settlementAssetID {
					continue
				}
				switch acct.Type {
				case proto.AccountType_ACCOUNT_TYPE_GENERAL:
					b.balanceGeneral = acct.Balance
				case proto.AccountType_ACCOUNT_TYPE_MARGIN:
					b.balanceMargin = acct.Balance
				case proto.AccountType_ACCOUNT_TYPE_BOND:
					b.balanceBond = acct.Balance
				}
			case eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA:
				b.marketData = event.GetMarketData()
				b.currentPrice = b.marketData.MarkPrice
			}
		}
	}
	// We have stopped so flag to the main app we have lost connection
	b.eventStreamLive = false
}

func (b *Bot) subscribePositions() error {
	req := &api.PositionsSubscribeRequest{
		MarketId: b.market.Id,
		PartyId:  b.walletPubKey,
	}
	stream, err := b.node.PositionsSubscribe(req)
	if err != nil {
		return errors.Wrap(err, "Failed to subscribe to positions")
	}

	// Run in background and process messages
	b.positionStreamLive = true
	go b.processPositions(stream)
	return nil
}

func (b *Bot) processPositions(stream api.TradingDataService_PositionsSubscribeClient) {
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
		b.openVolume = o.GetPosition().OpenVolume
	}
	// Let the app know we have stopped receiving position updates
	b.positionStreamLive = false
}
