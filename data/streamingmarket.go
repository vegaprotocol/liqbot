package data

import (
	"fmt"
	"io"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"
)

type Market struct {
	node              DataNode
	log               *log.Entry
	walletPubKey      string
	stakeLinkingCh    chan struct{}
	proposalIDCh      chan string
	proposalEnactedCh chan string
}

func NewMarket(node DataNode, walletPubKey string) *Market {
	return &Market{
		node:              node,
		log:               log.WithField("module", "MarketStreamer"),
		stakeLinkingCh:    make(chan struct{}),
		proposalIDCh:      make(chan string),
		proposalEnactedCh: make(chan string),
		walletPubKey:      walletPubKey,
	}
}

func (m *Market) Subscribe() error {
	if err := m.subscribeToStakeLinkingEvents(); err != nil {
		return fmt.Errorf("failed to subscribe to stake linking events: %w", err)
	}

	if err := m.subscribeToProposalEvents(); err != nil {
		return fmt.Errorf("failed to subscribe to proposal events: %w", err)
	}

	return nil
}

func (m *Market) subscribeToStakeLinkingEvents() error {
	req := &dataapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_STAKE_LINKING},
	}

	proc := func(event *eventspb.BusEvent) bool {
		if stake := event.GetStakeLinking(); stake.Party == m.walletPubKey && stake.Status == eventspb.StakeLinking_STATUS_ACCEPTED {
			go func() {
				m.stakeLinkingCh <- struct{}{}
				close(m.stakeLinkingCh)
			}()
			m.log.Debug("closing stake linking event stream")
			return true
		}
		return false
	}

	if err := m.processEvents(req, proc); err != nil {
		return fmt.Errorf("failed to process stake linking events: %w", err)
	}

	return nil
}

func (m *Market) WaitForStakeLinking() error {
	select {
	case <-m.stakeLinkingCh:
		return nil
	case <-time.NewTimer(time.Second * 40).C:
		return fmt.Errorf("timeout waiting for stake linking")
	}
}

func (m *Market) subscribeToProposalEvents() error {
	req := &dataapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	proc := func(event *eventspb.BusEvent) bool {
		switch event.GetProposal().State {
		case vega.Proposal_STATE_OPEN:
			go func() {
				m.proposalIDCh <- event.GetProposal().Id
				close(m.proposalIDCh)
			}()
		case vega.Proposal_STATE_ENACTED:
			go func() {
				m.proposalEnactedCh <- event.GetProposal().Id
				m.log.Debug("closing market proposal event stream")
				close(m.proposalEnactedCh)
			}()
			return true
		}
		return false
	}

	if err := m.processEvents(req, proc); err != nil {
		return fmt.Errorf("failed to process proposal events: %w", err)
	}

	return nil
}

func (m *Market) WaitForProposalID() (string, error) {
	select {
	case id := <-m.proposalIDCh:
		m.log.WithFields(log.Fields{
			"proposalID": id,
		}).Info("Received proposal ID")
		return id, nil
	case <-time.NewTimer(time.Second * 15).C:
		return "", fmt.Errorf("timeout waiting for proposal ID")
	}
}

func (m *Market) WaitForProposalEnacted(pID string) error {
	select {
	case id := <-m.proposalEnactedCh:
		if id == pID {
			m.log.WithFields(log.Fields{
				"proposalID": id,
			}).Info("Proposal was enacted")
			return nil
		}
	case <-time.NewTimer(time.Second * 25).C:
		return fmt.Errorf("timeout waiting for proposal to be enacted")
	}

	return nil
}

func (m *Market) processEvents(request *dataapipb.ObserveEventBusRequest, cb func(event *eventspb.BusEvent) bool) error {
	// First we have to create the stream
	stream, err := m.node.ObserveEventBus()
	if err != nil {
		return fmt.Errorf("failed to subscribe to event bus data: %w", err)
	}

	// Then we subscribe to the data
	if err = stream.SendMsg(request); err != nil {
		return fmt.Errorf("unable to send event bus request on the stream: %w", err)
	}

	go func() {
		for {
			eb, err := stream.Recv()
			if err == io.EOF {
				m.log.Warning("event bus data: stream closed by server (EOF)")
				break
			}
			if err != nil {
				m.log.WithFields(log.Fields{
					"error": err,
				}).Warning("event bus data: stream closed")
				break
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
