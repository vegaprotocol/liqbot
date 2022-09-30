package data

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/util"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	"code.vegaprotocol.io/vega/protos/vega"
	coreapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
)

type account struct {
	log          *log.Entry
	node         DataNode
	stores       map[string]BalanceStore
	walletPubKey string
	busEvProc    busEventer

	mu              sync.Mutex
	waitingDeposits map[string]*num.Uint
}

func NewAccountStream(node DataNode) *account {
	return &account{
		log:             log.WithField("module", "AccountStreamer"),
		node:            node,
		waitingDeposits: make(map[string]*num.Uint),
	}
}

func (a *account) Init(pubKey string, pauseCh chan types.PauseSignal) {
	a.walletPubKey = pubKey
	a.busEvProc = newBusEventProcessor(a.node, WithPauseCh(pauseCh))
	a.stores = make(map[string]BalanceStore)
}

func (a *account) InitBalances(assetID string) (BalanceStore, error) {
	response, err := a.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: a.walletPubKey,
		Asset:   assetID,
	})
	if err != nil {
		return nil, err
	}

	if len(response.Accounts) == 0 {
		a.log.WithFields(log.Fields{
			"party": a.walletPubKey,
		}).Warning("Party has no accounts")
	}

	store := types.NewBalanceStore()
	a.stores[assetID] = store

	for _, acc := range response.Accounts {
		if err = a.setBalanceByType(acc); err != nil {
			a.log.WithFields(
				log.Fields{
					"error":       err.Error(),
					"accountType": acc.Type.String(),
				},
			).Error("failed to set account balance")
		}
	}

	// TODO: avoid goroutine per every asset, better use a list of assets in that one goroutine
	go a.subscribeToAccountEvents(assetID)

	return store, nil
}

func (a *account) subscribeToAccountEvents(assetID string) {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: a.walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			acct := event.GetAccount()
			// filter out any that are for different assets
			if acct.Asset != assetID {
				continue
			}

			if err := a.setBalanceByType(acct); err != nil {
				a.log.WithFields(
					log.Fields{
						"error":       err.Error(),
						"accountType": acct.Type.String(),
					},
				).Error("failed to set account balance")
			}
		}
		return false, nil
	}

	a.busEvProc.processEvents(context.Background(), "AccountData", req, proc)
}

// WaitForDepositFinalise is a blocking call that waits for the deposit finalize event to be received.
func (a *account) WaitForDepositFinalise(ctx context.Context, walletPubKey, assetID string, expectAmount *num.Uint, timeout time.Duration) error {
	if exist, ok := a.getWaitingDeposit(assetID); ok {
		if expectAmount.GT(exist) {
			a.setWaitingDeposit(assetID, expectAmount)
		}
		return nil
	}

	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_DEPOSIT,
		},
		PartyId: walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			dep := event.GetDeposit()
			// filter out any that are for different assets, or not finalized
			if dep.Asset != assetID || dep.Status != vega.Deposit_STATUS_FINALIZED {
				continue
			}

			gotAmount, overflow := num.UintFromString(dep.Amount, 10)
			if overflow {
				return false, fmt.Errorf("failed to parse deposit expectAmount %s", dep.Amount)
			}

			expect, ok := a.getWaitingDeposit(assetID)
			if !ok {
				expect = expectAmount.Clone()
				a.setWaitingDeposit(assetID, expect)
			}

			if gotAmount.GTE(expect) {
				a.log.WithFields(log.Fields{"amount": gotAmount}).Info("Deposit finalised")
				a.deleteWaitingDeposit(assetID)
				return true, nil
			}
		}
		return false, nil
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	a.busEvProc.processEvents(ctx, "DepositData", req, proc)
	return ctx.Err()
}

func (a *account) getWaitingDeposit(assetID string) (*num.Uint, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	req, ok := a.waitingDeposits[assetID]
	if ok {
		return req.Clone(), ok
	}
	return nil, false
}

func (a *account) setWaitingDeposit(assetID string, amount *num.Uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.waitingDeposits[assetID] = amount.Clone()
}

func (a *account) deleteWaitingDeposit(assetID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.waitingDeposits, assetID)
}

func (a *account) WaitForStakeLinking(pubKey string) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_STAKE_LINKING},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			stake := event.GetStakeLinking()
			if stake.Party == pubKey && stake.Status == eventspb.StakeLinking_STATUS_ACCEPTED {
				a.log.WithFields(log.Fields{
					"stakeID": stake.Id,
				}).Info("Received stake linking")
				return true, nil // stop processing
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*45)
	defer cancel()

	a.busEvProc.processEvents(ctx, "StakeLinking", req, proc)
	return ctx.Err()
}

func (a *account) setBalanceByType(account *vega.Account) error {
	balance, err := util.ConvertUint256(account.Balance)
	if err != nil {
		return fmt.Errorf("failed to convert account balance: %w", err)
	}

	a.stores[account.Asset].BalanceSet(types.SetBalanceByType(account.Type, balance))
	return nil
}
