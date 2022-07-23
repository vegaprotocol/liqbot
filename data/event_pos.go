package data

import (
	v1 "code.vegaprotocol.io/protos/data-node/api/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
)

func newPosEventProcessor(node DataNode, pauseCh chan types.PauseSignal) *posEventProcessor {
	return &posEventProcessor{
		node:    node,
		log:     log.WithFields(log.Fields{"module": "EventProcessor", "event": "Positions"}),
		pauseCh: pauseCh,
	}
}

type (
	posEventProcessor = eventProcessor[*v1.PositionsSubscribeRequest, *v1.PositionsSubscribeResponse, posStreamGetter]
	posStreamer       = streamer[*v1.PositionsSubscribeResponse]
	posStreamGetter   struct{}
)

func (p posStreamGetter) getStream(node DataNode, req *v1.PositionsSubscribeRequest) (posStreamer, error) {
	s, err := node.PositionsSubscribe(req)
	return interface {
		Recv() (*v1.PositionsSubscribeResponse, error)
	}(s), err
}
