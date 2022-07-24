package data

import (
	"fmt"
	"time"

	"code.vegaprotocol.io/protos/vega"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
)

type Market struct {
	node              DataNode
	log               *log.Entry
	walletPubKey      string
	busEvProc         busEventer
	stakeLinkingCh    chan struct{}
	proposalIDCh      chan string
	proposalEnactedCh chan string
}

func NewMarket(node DataNode, walletPubKey string, pauseCh chan types.PauseSignal) *Market {
	return &Market{
		node:              node,
		log:               log.WithField("module", "MarketStreamer"),
		busEvProc:         newBusEventProcessor(node, pauseCh),
		stakeLinkingCh:    make(chan struct{}),
		proposalIDCh:      make(chan string),
		proposalEnactedCh: make(chan string),
		walletPubKey:      walletPubKey,
	}
}

func (m *Market) Subscribe() {
	m.subscribeToStakeLinkingEvents()
	m.subscribeToProposalEvents()
}

func (m *Market) subscribeToStakeLinkingEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_STAKE_LINKING},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) error {
		for _, event := range rsp.GetEvents() {
			if stake := event.GetStakeLinking(); stake.Party == m.walletPubKey && stake.Status == eventspb.StakeLinking_STATUS_ACCEPTED {
				select {
				case m.stakeLinkingCh <- struct{}{}:
					m.log.Debug("Closing stake linking event stream")
					close(m.stakeLinkingCh)
				}
				return nil
			}
		}
		return nil
	}

	go m.busEvProc.processEvents("StakeLinking", req, proc)
}

func (m *Market) WaitForStakeLinking() error {
	select {
	case <-m.stakeLinkingCh:
		return nil
	case <-time.NewTimer(time.Second * 40).C:
		return fmt.Errorf("timeout waiting for stake linking")
	}
}

func (m *Market) subscribeToProposalEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) error {
		for _, event := range rsp.GetEvents() {
			switch event.GetProposal().State {
			case vega.Proposal_STATE_OPEN:
				select {
				case m.proposalIDCh <- event.GetProposal().Id:
					close(m.proposalIDCh)
				default:
				}
			case vega.Proposal_STATE_ENACTED:
				select {
				case m.proposalEnactedCh <- event.GetProposal().Id:
					m.log.Debug("Closing market proposal event stream")
					close(m.proposalEnactedCh)
				default:
				}
			}
		}
		return nil
	}

	go m.busEvProc.processEvents("Proposals", req, proc)
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
