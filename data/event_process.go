package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	log "github.com/sirupsen/logrus"

	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/types"
)

type busEventProcessor struct {
	node    DataNode
	log     *log.Entry
	pauseCh chan types.PauseSignal
}

func newBusEventProcessor(node DataNode, pauseCh chan types.PauseSignal) *busEventProcessor {
	return &busEventProcessor{
		node:    node,
		log:     log.WithFields(log.Fields{"module": "EventProcessor", "event": "EventBus"}),
		pauseCh: pauseCh,
	}
}

func (b *busEventProcessor) processEvents(
	ctx context.Context,
	name string,
	req *coreapipb.ObserveEventBusRequest,
	process func(*coreapipb.ObserveEventBusResponse) (bool, error),
) {
	defer b.log.WithFields(log.Fields{
		"name": name,
	}).Debug("Stopping event processor")

	var stop bool
	for s := b.mustGetStream(ctx, name, req); !stop; {
		select {
		case <-ctx.Done():
			return
		default:
			if s == nil {
				return
			}

			rsp, err := s.Recv()
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return
				}

				b.log.WithFields(
					log.Fields{
						"error": err.Error(),
						"name":  name,
					},
				).Warningf("Stream closed, resubscribing...")

				b.pause(true, name)
				s = b.mustGetStream(ctx, name, req)
				b.pause(false, name)
				continue
			}

			stop, err = process(rsp)
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
					"name":  name,
				}).Warning("Unable to process event")
			}
		}
	}
}

func (b *busEventProcessor) mustGetStream(
	ctx context.Context,
	name string,
	req *coreapipb.ObserveEventBusRequest,
) coreapipb.CoreService_ObserveEventBusClient {
	var (
		s   coreapipb.CoreService_ObserveEventBusClient
		err error
	)

	attempt := 0
	sleepTime := time.Second * 3

	for s, err = b.getStream(ctx, req); err != nil; s, err = b.getStream(ctx, req) {
		if errors.Unwrap(err).Error() == e.ErrConnectionNotReady.Error() {
			b.log.WithFields(log.Fields{
				"name":    name,
				"error":   err.Error(),
				"attempt": attempt,
			}).Warning("Node is not ready, reconnecting")

			b.node.MustDialConnection(ctx)

			b.log.WithFields(log.Fields{
				"name":    name,
				"attempt": attempt,
			}).Debug("Node reconnected, reattempting to subscribe to stream")
		} else if ctx.Err() == context.DeadlineExceeded {
			b.log.WithFields(log.Fields{
				"name": name,
			}).Warning("Deadline exceeded. Stopping event processor")

			break
		} else {
			attempt++

			b.log.WithFields(log.Fields{
				"name":    name,
				"error":   err.Error(),
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

func (b *busEventProcessor) getStream(ctx context.Context, req *coreapipb.ObserveEventBusRequest) (coreapipb.CoreService_ObserveEventBusClient, error) {
	s, err := b.node.ObserveEventBus(ctx)
	if err != nil {
		return nil, err
	}
	// Then we subscribe to the data
	if err = s.SendMsg(req); err != nil {
		return nil, fmt.Errorf("failed to send event bus request for stream: %w", err)
	}
	return s, nil
}

func (b *busEventProcessor) pause(p bool, name string) {
	select {
	case b.pauseCh <- types.PauseSignal{From: name, Pause: p}:
	default:
	}
}
