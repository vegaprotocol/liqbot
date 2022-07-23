package data

import (
	"fmt"

	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
)

func newBusEventProcessor(node DataNode, pauseCh chan types.PauseSignal) *busEventProcessor {
	return &busEventProcessor{
		node:    node,
		log:     log.WithFields(log.Fields{"module": "EventProcessor", "event": "EventBus"}),
		pauseCh: pauseCh,
	}
}

type (
	busEventProcessor = eventProcessor[*coreapipb.ObserveEventBusRequest, *coreapipb.ObserveEventBusResponse, busStreamGetter]
	busStreamer       = streamer[*coreapipb.ObserveEventBusResponse]
	busStreamGetter   struct{}
)

func (b busStreamGetter) getStream(node DataNode, req *coreapipb.ObserveEventBusRequest) (busStreamer, error) {
	s, err := node.ObserveEventBus()
	if err != nil {
		return nil, err
	}
	// Then we subscribe to the data
	if err = s.SendMsg(req); err != nil {
		return nil, fmt.Errorf("failed to send event bus request for stream: %w", err)
	}

	return interface {
		Recv() (*coreapipb.ObserveEventBusResponse, error)
	}(s), nil
}
