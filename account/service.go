package account

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
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

func (a *Service) EnsureBalance(ctx context.Context, assetID string, balanceFn func(types.Balance) *num.Uint, targetAmount *num.Uint, from string) error {
	store, err := a.getStore(ctx, assetID)
	if err != nil {
		return err
	}

	// or liquidity provision and placing orders, we need only General account balance
	// for liquidity increase, we need both Bond and General account balance
	balance := balanceFn(store.Balance())

	a.log.WithFields(
		log.Fields{
			"name":    a.name,
			"partyId": a.pubKey,
			"balance": balance.String(),
			"asset":   assetID,
		}).Debugf("%s: Total account balance", from)

	if balance.GTE(targetAmount) {
		return nil
	}

	a.log.WithFields(
		log.Fields{
			"name":         a.name,
			"partyId":      a.pubKey,
			"asset":        assetID,
			"balance":      balance.String(),
			"targetAmount": targetAmount.String(),
		}).Debugf("%s: Account balance is less than target amount, depositing...", from)

	if err := a.coinProvider.TopUpAsync(ctx, a.name, a.pubKey, assetID, targetAmount); err != nil {
		return fmt.Errorf("failed to top up: %w", err)
	}

	defer a.log.WithFields(log.Fields{"name": a.name}).Debugf("%s: Top-up complete", from)

	a.log.WithFields(log.Fields{"name": a.name}).Debugf("%s: Waiting for top-up...", from)

	if err = a.accountStream.WaitForTopUpToFinalise(ctx, a.pubKey, assetID, targetAmount, 0); err != nil {
		return fmt.Errorf("failed to finalise deposit: %w", err)
	}

	return nil
}

func (a *Service) EnsureStake(ctx context.Context, receiverName, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error {
	if receiverPubKey == "" {
		return fmt.Errorf("receiver public key is empty")
	}

	stake, err := a.accountStream.GetStake(ctx)
	if err != nil {
		return err
	}

	a.log.WithFields(
		log.Fields{
			"name":    a.name,
			"partyId": a.pubKey,
			"stake":   stake.String(),
		}).Debugf("%s: Total account stake balance", from)

	if stake.GT(targetAmount) {
		return nil
	}

	a.log.WithFields(
		log.Fields{
			"name":           a.name,
			"receiverName":   receiverName,
			"receiverPubKey": receiverPubKey,
			"partyId":        a.pubKey,
			"stake":          stake.String(),
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

	if err = a.accountStream.WaitForStakeLinking(ctx, receiverPubKey); err != nil {
		return fmt.Errorf("failed to finalise stake: %w", err)
	}

	return nil
}

func (a *Service) StakeAsync(ctx context.Context, receiverPubKey, assetID string, amount *num.Uint) error {
	return a.coinProvider.StakeAsync(ctx, receiverPubKey, assetID, amount)
}

func (a *Service) Balance(ctx context.Context) types.Balance {
	store, err := a.getStore(ctx, a.assetID)
	if err != nil {
		a.log.WithError(err).Error("failed to get balance store")
		return types.Balance{}
	}
	return store.Balance()
}

func (a *Service) getStore(ctx context.Context, assetID string) (data.BalanceStore, error) {
	var err error

	store, ok := a.stores[assetID]
	if !ok {
		store, err = a.accountStream.GetBalances(ctx, assetID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialise balances for '%s': %w", assetID, err)
		}

		a.stores[assetID] = store
	}

	return store, nil
}
