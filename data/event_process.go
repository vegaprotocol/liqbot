package data

import (
	"errors"
	"time"

	log "github.com/sirupsen/logrus"

	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/types"
)

type eventProcessor[req any, rsp any, _ streamGetter[req, rsp]] struct {
	node    DataNode
	log     *log.Entry
	pauseCh chan types.PauseSignal
}

type streamGetter[req any, rsp any] interface {
	getStream(DataNode, req) (streamer[rsp], error)
}

type streamer[resp any] interface {
	Recv() (resp, error)
}

func (b *eventProcessor[request, response, _]) processEvents(name string, req request, process func(response) error) {
	for s := b.mustGetStream(name, req); ; {
		rsp, err := s.Recv()
		if err != nil {
			b.log.WithFields(
				log.Fields{
					"error": err,
					"name":  name,
				},
			).Warningf("Stream closed, resubscribing...")

			b.pause(true, name)
			s = b.mustGetStream(name, req)
			b.pause(false, name)
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

func (b *eventProcessor[request, response, getter]) mustGetStream(name string, req request) streamer[response] {
	var (
		sg  getter
		s   streamer[response]
		err error
	)

	attempt := 0
	sleepTime := time.Second * 3

	for s, err = sg.getStream(b.node, req); err != nil; s, err = sg.getStream(b.node, req) {
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

func (b *eventProcessor[_, _, _]) pause(p bool, name string) {
	select {
	case b.pauseCh <- types.PauseSignal{From: name, Pause: p}:
	default:
	}
}
