package data

import (
	"errors"
	"fmt"
	"time"

	v1 "code.vegaprotocol.io/protos/data-node/api/v1"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	log "github.com/sirupsen/logrus"

	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/types"
)

type eventProcessor[req any, rsp any] struct {
	node      DataNode
	log       *log.Entry
	pauseCh   chan types.PauseSignal
	getStream func(req) (streamer[rsp], error)
}

func newBusEventProcessor(node DataNode, pauseCh chan types.PauseSignal) *eventProcessor[*coreapipb.ObserveEventBusRequest, *coreapipb.ObserveEventBusResponse] {
	return &eventProcessor[*coreapipb.ObserveEventBusRequest, *coreapipb.ObserveEventBusResponse]{
		node:    node,
		log:     log.WithFields(log.Fields{"module": "EventProcessor", "event": "EventBus"}),
		pauseCh: pauseCh,
		getStream: func(r *coreapipb.ObserveEventBusRequest) (streamer[*coreapipb.ObserveEventBusResponse], error) {
			//always get a new stream
			s, err := node.ObserveEventBus()
			if err != nil {
				return nil, err
			}
			// Then we subscribe to the data
			if err := s.SendMsg(r); err != nil {
				return nil, fmt.Errorf("failed to send event bus request for stream: %w", err)
			}

			return &busStreamer[*coreapipb.ObserveEventBusResponse]{s}, nil
		},
	}
}

func newPosEventProcessor(node DataNode, pauseCh chan types.PauseSignal) *eventProcessor[*v1.PositionsSubscribeRequest, *v1.PositionsSubscribeResponse] {
	return &eventProcessor[*v1.PositionsSubscribeRequest, *v1.PositionsSubscribeResponse]{
		node:    node,
		log:     log.WithFields(log.Fields{"module": "EventProcessor", "event": "Positions"}),
		pauseCh: pauseCh,
		getStream: func(req *v1.PositionsSubscribeRequest) (streamer[*v1.PositionsSubscribeResponse], error) {
			s, err := node.PositionsSubscribe(req)
			return &posStreamer[*v1.PositionsSubscribeResponse]{s}, err
		},
	}
}

func (b *eventProcessor[request, response]) processEvents(name string, req request, process func(response) error) {
	for s := b.mustGetStream(name, req); ; {
		rsp, err := s.Recv()
		if err != nil {
			b.log.WithFields(
				log.Fields{
					"error": err,
					"name":  name,
				},
			).Warningf("Stream closed, resubscribing...")

			b.pauseBot(true, name)
			s = b.mustGetStream(name, req)
			b.pauseBot(false, name)
			continue
		}

		if err = process(rsp); err != nil {
			b.log.WithFields(log.Fields{
				"error": err,
				"name":  name,
			}).Warning("Unable to process event")
		}
	}
}

func (b *eventProcessor[request, response]) mustGetStream(name string, req request) streamer[response] {
	var (
		s   streamer[response]
		err error
	)

	attempt := 0
	sleepTime := time.Second * 3

	for s, err = b.getStream(req); err != nil; s, err = b.getStream(req) {
		if errors.Unwrap(err).Error() == e.ErrConnectionNotReady.Error() {
			b.log.WithFields(log.Fields{
				"name":    name,
				"error":   err,
				"attempt": attempt,
			}).Warning("Node is not ready, reconnecting")

			<-b.node.DialConnection()

			b.log.WithFields(log.Fields{
				"name":    name,
				"attempt": attempt,
			}).Debug("Node reconnected, reattempting to subscribe to stream")
		} else {
			attempt++

			b.log.WithFields(log.Fields{
				"name":    name,
				"error":   err,
				"attempt": attempt,
			}).Errorf("Failed to subscribe to stream, retrying in %s...", sleepTime)

			time.Sleep(sleepTime)
		}
	}

	b.log.WithFields(log.Fields{
		"name":    name,
		"attempt": attempt,
	}).Debug("Stream subscribed")

	return s
}

func (b *eventProcessor[_, _]) pauseBot(p bool, name string) {
	select {
	case b.pauseCh <- types.PauseSignal{From: name, Pause: p}:
	default:
	}
}

type streamer[resp any] interface {
	Recv() (resp, error)
}

type busStreamer[resp any] struct {
	coreapipb.CoreService_ObserveEventBusClient
}

func (s *busStreamer[resp]) Recv() (*coreapipb.ObserveEventBusResponse, error) {
	return s.CoreService_ObserveEventBusClient.Recv()
}

type posStreamer[resp any] struct {
	v1.TradingDataService_PositionsSubscribeClient
}

func (s *posStreamer[resp]) Recv() (*v1.PositionsSubscribeResponse, error) {
	return s.TradingDataService_PositionsSubscribeClient.Recv()
}
