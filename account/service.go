package account

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

type Service struct {
	name          string
	pubKey        string
	assetID       string
	stores        map[string]data.BalanceStore
	accountStream accountStream
	coinProvider  types.CoinProvider
	log           *log.Entry
}

func NewAccountService(name, assetID string, accountStream accountStream, coinProvider types.CoinProvider) *Service {
	return &Service{
		name:          name,
		assetID:       assetID,
		accountStream: accountStream,
		coinProvider:  coinProvider,
		log:           log.WithField("component", "AccountService"),
	}
}

func (a *Service) Init(pubKey string, pauseCh chan types.PauseSignal) {
	a.stores = make(map[string]data.BalanceStore)
	a.pubKey = pubKey
	a.accountStream.Init(pubKey, pauseCh)
}

func (a *Service) EnsureBalance(ctx context.Context, assetID string, targetAmount *num.Uint, from string) error {
	store, err := a.getStore(assetID)
	if err != nil {
		return err
	}

	balanceTotal := store.Balance().Total() // TODO: should it be total balance?

	a.log.WithFields(
		log.Fields{
			"name":         a.name,
			"partyId":      a.pubKey,
			"balanceTotal": balanceTotal.String(),
		}).Debugf("%s: Total account balance", from)

	if balanceTotal.GTE(targetAmount) {
		return nil
	}

	a.log.WithFields(
		log.Fields{
			"name":         a.name,
			"partyId":      a.pubKey,
			"balanceTotal": balanceTotal.String(),
			"targetAmount": targetAmount.String(),
		}).Debugf("%s: Account balance is less than target amount, depositing...", from)

	evtType, err := a.coinProvider.TopUpAsync(ctx, a.name, a.pubKey, assetID, targetAmount)
	if err != nil {
		return fmt.Errorf("failed to top up: %w", err)
	}

	a.log.WithFields(log.Fields{"name": a.name}).Debugf("%s: Waiting for top-up...", from)

	if err = a.accountStream.WaitForTopUpToFinalise(ctx, evtType, a.pubKey, assetID, targetAmount, 0); err != nil {
		return fmt.Errorf("failed to finalise deposit: %w", err)
	}

	a.log.WithFields(log.Fields{"name": a.name}).Debugf("%s: Top-up complete", from)

	return nil
}

// TODO: DRY
func (a *Service) EnsureStake(ctx context.Context, receiverName, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error {
	if receiverPubKey == "" {
		return fmt.Errorf("receiver public key is empty")
	}

	store, err := a.getStore(assetID)
	if err != nil {
		return err
	}

	// TODO: how the hell do we check for stake balance??
	balanceTotal := store.Balance().Total()

	a.log.WithFields(
		log.Fields{
			"name":         a.name,
			"partyId":      a.pubKey,
			"balanceTotal": balanceTotal.String(),
		}).Debugf("%s: Total account stake balance", from)

	if balanceTotal.GT(targetAmount) {
		return nil
	}

	a.log.WithFields(
		log.Fields{
			"name":           a.name,
			"receiverName":   receiverName,
			"receiverPubKey": receiverPubKey,
			"partyId":        a.pubKey,
			"balanceTotal":   balanceTotal.String(),
			"targetAmount":   targetAmount.String(),
		}).Debugf("%s: Account Stake balance is less than target amount, staking...", from)

	if err = a.coinProvider.StakeAsync(ctx, receiverPubKey, assetID, targetAmount); err != nil {
		return fmt.Errorf("failed to stake: %w", err)
	}

	a.log.WithFields(log.Fields{
		"name":           a.name,
		"receiverName":   receiverName,
		"receiverPubKey": receiverPubKey,
		"partyId":        a.pubKey,
		"targetAmount":   targetAmount.String(),
	}).Debugf("%s: Waiting for staking...", from)

	if err = a.accountStream.WaitForStakeLinking(receiverPubKey); err != nil {
		return fmt.Errorf("failed to finalise stake: %w", err)
	}

	return nil
}

func (a *Service) StakeAsync(ctx context.Context, receiverPubKey, assetID string, amount *num.Uint) error {
	return a.coinProvider.StakeAsync(ctx, receiverPubKey, assetID, amount)
}

func (a *Service) Balance() types.Balance {
	store, err := a.getStore(a.assetID)
	if err != nil {
		a.log.WithError(err).Error("failed to get balance store")
		return types.Balance{}
	}
	return store.Balance()
}

func (a *Service) getStore(assetID string) (_ data.BalanceStore, err error) {
	store, ok := a.stores[assetID]
	if !ok {
		store, err = a.accountStream.GetBalances(assetID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialise balances for '%s': %w", assetID, err)
		}

		a.stores[assetID] = store
	}

	return store, nil
}
