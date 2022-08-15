package data

import (
	"context"
	"time"

	"code.vegaprotocol.io/protos/vega"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types/num"
)

type deposit struct {
	log          *log.Entry
	busEvProc    busEventer
	walletPubKey string
}

func NewDepositStream(node busStreamer, pubKey string) *deposit {
	return &deposit{
		log:          log.WithField("module", "DepositStreamer"),
		busEvProc:    newBusEventProcessor(node, nil),
		walletPubKey: pubKey,
	}
}

// WaitForDepositFinalize is a blocking call that waits for the deposit finalize event to be received.
func (d *deposit) WaitForDepositFinalize(ctx context.Context, settlementAssetID string, amount *num.Uint, timeout time.Duration) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_DEPOSIT,
		},
		PartyId: d.walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			dep := event.GetDeposit()
			// filter out any that are for different assets, or not finalized
			if dep.Asset != settlementAssetID || dep.Status != vega.Deposit_STATUS_FINALIZED {
				continue
			}

			if dep.Amount == amount.String() {
				d.log.WithFields(log.Fields{"amount": amount}).Info("Deposit finalized")
				return true, nil
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d.busEvProc.processEvents(ctx, "DepositData", req, proc)
	return ctx.Err()
}
