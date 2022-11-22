package data

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	coreapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
)

type account struct {
	name          string
	log           *log.Entry
	node          DataNode
	balanceStores *balanceStores
	walletPubKey  string
	busEvProc     busEventer

	mu              sync.Mutex
	waitingDeposits map[string]*num.Uint
}

func NewAccountStream(name string, node DataNode) *account {
	return &account{
		name:            name,
		log:             log.WithField("component", "AccountStreamer"),
		node:            node,
		waitingDeposits: make(map[string]*num.Uint),
	}
}

func (a *account) Init(pubKey string, pauseCh chan types.PauseSignal) {
	a.walletPubKey = pubKey
	a.busEvProc = newBusEventProcessor(a.node, WithPauseCh(pauseCh))
	a.balanceStores = &balanceStores{
		balanceStores: make(map[string]BalanceStore),
	}

	a.subscribeToAccountEvents()
}

func (a *account) GetBalances(ctx context.Context, assetID string) (BalanceStore, error) {
	if store, ok := a.balanceStores.get(assetID); ok {
		return store, nil
	}

	accounts, err := a.node.PartyAccounts(ctx, &dataapipb.ListAccountsRequest{
		Filter: &dataapipb.AccountFilter{
			PartyIds: []string{a.walletPubKey},
			AssetId:  assetID,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		a.log.WithFields(log.Fields{
			"name":    a.name,
			"partyId": a.walletPubKey,
		}).Warningf("Party has no accounts for asset %s", assetID)
	}

	store := types.NewBalanceStore()
	a.balanceStores.set(assetID, store)

	for _, acc := range accounts {
		a.log.WithFields(log.Fields{
			"name":        a.name,
			"partyId":     a.walletPubKey,
			"accountType": acc.Type.String(),
			"balance":     acc.Balance,
			"assetID":     acc.Asset,
		}).Debug("Setting initial account balance")

		if err = a.setBalanceByType(acc.Type, acc.Balance, store); err != nil {
			a.log.WithFields(
				log.Fields{
					"error":       err.Error(),
					"accountType": acc.Type.String(),
				},
			).Error("failed to set account balance")
		}
	}

	return store, nil
}

func (a *account) GetStake(ctx context.Context) (*num.Uint, error) {
	partyStakeResp, err := a.node.PartyStake(ctx, &dataapipb.GetStakeRequest{
		PartyId: a.walletPubKey,
	})
	if err != nil {
		return nil, err
	}

	stake, overflow := num.UintFromString(partyStakeResp.CurrentStakeAvailable, 10)
	if overflow {
		return nil, fmt.Errorf("failed to convert stake to uint: %w", err)
	}

	return stake, nil
}

func (a *account) subscribeToAccountEvents() {
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
			store, ok := a.balanceStores.get(acct.Asset)
			if !ok {
				continue
			}

			if err := a.setBalanceByType(acct.Type, acct.Balance, store); err != nil {
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

	a.busEvProc.processEvents(context.Background(), "AccountData: "+a.name, req, proc)
}

func (a *account) setBalanceByType(accountType vega.AccountType, balanceStr string, store BalanceStore) error {
	balance, err := num.ConvertUint256(balanceStr)
	if err != nil {
		return fmt.Errorf("failed to convert account balance: %w", err)
	}

	store.BalanceSet(types.SetBalanceByType(accountType, balance))
	return nil
}

// WaitForTopUpToFinalise is a blocking call that waits for the top-up finalise event to be received.
func (a *account) WaitForTopUpToFinalise(
	ctx context.Context,
	walletPubKey,
	assetID string,
	expectAmount *num.Uint,
	timeout time.Duration,
) error {
	if exist, ok := a.getWaitingDeposit(assetID); ok {
		if !expectAmount.EQ(exist) {
			a.setWaitingDeposit(assetID, expectAmount)
		}
		return nil
	}

	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_DEPOSIT,
			eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER,
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			var (
				status   string
				partyId  string
				asset    string
				balance  string
				okStatus []string
			)
			switch event.Type {
			case eventspb.BusEventType_BUS_EVENT_TYPE_DEPOSIT:
				depEvt := event.GetDeposit()
				status = depEvt.Status.String()
				partyId = depEvt.PartyId
				asset = depEvt.Asset
				okStatus = []string{
					vega.Deposit_STATUS_FINALIZED.String(),
					vega.Deposit_STATUS_OPEN.String(),
				}
			case eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER:
				depEvt := event.GetTransfer()
				status = depEvt.Status.String()
				partyId = depEvt.To
				asset = depEvt.Asset
				okStatus = []string{
					eventspb.Transfer_STATUS_DONE.String(),
					eventspb.Transfer_STATUS_PENDING.String(),
				}
			case eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT:
				accEvt := event.GetAccount()
				partyId = accEvt.Owner
				asset = accEvt.Asset
				// we only care about the balance of the general account
				if accEvt.Type == vega.AccountType_ACCOUNT_TYPE_GENERAL {
					balance = accEvt.GetBalance()
				}
			}

			// filter out any that are for different assets, or not finalized
			if partyId != walletPubKey || asset != assetID {
				continue
			}

			// if it's a deposit or transfer event, check if it failed
			if status != "" && !slices.Contains(okStatus, status) {
				return true, fmt.Errorf("transfer %s failed: %s", event.Id, status)
			}

			a.log.WithFields(log.Fields{
				"account.name":  a.name,
				"event.partyID": partyId,
				"event.assetID": asset,
				"event.balance": balance,
				"event.status":  status,
			}).Debugf("Received %s event", event.Type.String())

			// only check the deposited amount for account events
			if event.Type != eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT {
				continue
			}

			gotAmount, err := num.ConvertUint256(balance)
			if err != nil {
				return false, fmt.Errorf("failed to parse top-up expectAmount %s: %w", balance, err)
			}

			expect, ok := a.getWaitingDeposit(assetID)
			if !ok {
				expect = expectAmount.Clone()
				a.setWaitingDeposit(assetID, expect)
			}

			if gotAmount.GTE(expect) {
				a.log.WithFields(log.Fields{
					"name":    a.name,
					"partyId": walletPubKey,
					"balance": gotAmount.String(),
				}).Info("TopUp finalised")
				a.deleteWaitingDeposit(assetID)
				return true, nil
			} else {
				a.log.WithFields(log.Fields{
					"name":         a.name,
					"partyId":      a.walletPubKey,
					"gotAmount":    gotAmount.String(),
					"targetAmount": expect.String(),
				}).Info("Received funds, but balance is less than expected")
				// if we received fewer funds than expected, keep waiting (e.g. faucet tops-up in multiple iterations)
			}
		}
		return false, nil
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	errCh := a.busEvProc.processEvents(ctx, "TopUpData: "+a.name, req, proc)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for top-up event")
	}
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

func (a *account) WaitForStakeLinking(ctx context.Context, pubKey string) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{eventspb.BusEventType_BUS_EVENT_TYPE_STAKE_LINKING},
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.GetEvents() {
			stake := event.GetStakeLinking()
			if stake.Party != pubKey {
				continue
			}

			if stake.Status != eventspb.StakeLinking_STATUS_ACCEPTED {
				if stake.Status == eventspb.StakeLinking_STATUS_PENDING {
					continue
				} else {
					return true, fmt.Errorf("stake linking failed: %s", stake.Status.String())
				}
			}
			a.log.WithFields(log.Fields{
				"name":    a.name,
				"partyId": stake.Party,
				"stakeID": stake.Id,
			}).Info("Received stake linking")
			return true, nil
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*450)
	defer cancel()

	errCh := a.busEvProc.processEvents(ctx, "StakeLinking: "+a.name, req, proc)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for top-up event")
	}
}

type balanceStores struct {
	mu            sync.Mutex
	balanceStores map[string]BalanceStore
}

func (b *balanceStores) get(assetID string) (BalanceStore, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	store, ok := b.balanceStores[assetID]
	return store, ok
}

func (b *balanceStores) set(assetID string, store BalanceStore) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.balanceStores[assetID] = store
}
