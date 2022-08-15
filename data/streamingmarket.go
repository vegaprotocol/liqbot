package data

import (
	"context"
	"time"

	"code.vegaprotocol.io/protos/vega"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
)

type market struct {
	node         DataNode
	log          *log.Entry
	walletPubKey string
	busEvProc    busEventer
}

func NewMarketStream(node DataNode) *market {
	return &market{
		node: node,
		log:  log.WithField("module", "MarketStreamer"),
	}
}

func (m *market) Setup(walletPubKey string, pauseCh chan types.PauseSignal) {
	m.walletPubKey = walletPubKey
	m.busEvProc = newBusEventProcessor(m.node, pauseCh)
}

func (m *market) WaitForStakeLinking() error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_STAKE_LINKING},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			if stake := event.GetStakeLinking(); stake.Party == m.walletPubKey && stake.Status == eventspb.StakeLinking_STATUS_ACCEPTED {
				m.log.WithFields(log.Fields{
					"stakeID": stake.Id,
				}).Info("Received stake linking")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*45)
	defer cancel()

	m.busEvProc.processEvents(ctx, "StakeLinking", req, proc)
	return ctx.Err()
}

func (m *market) WaitForProposalID() (string, error) {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	var proposalID string

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.State == vega.Proposal_STATE_OPEN {
				proposalID = proposal.Id

				m.log.WithFields(log.Fields{
					"proposalID": proposalID,
				}).Info("Received proposal ID")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	m.busEvProc.processEvents(ctx, "Proposals", req, proc)
	return proposalID, ctx.Err()
}

func (m *market) WaitForProposalEnacted(pID string) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			proposal := event.GetProposal()

			if proposal.State == vega.Proposal_STATE_ENACTED && proposal.Id == pID {
				m.log.WithFields(
					log.Fields{
						"proposalID": proposal.Id,
					}).Debug("Proposal was enacted")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	m.busEvProc.processEvents(ctx, "Proposals", req, proc)
	return ctx.Err()
}
