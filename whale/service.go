package whale

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	vtypes "code.vegaprotocol.io/vega/core/types"
	commV1 "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	"code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type Service struct {
	node    dataNode
	wallet  walletClient
	account accountService

	walletName       string
	walletPassphrase string
	walletPubKey     string
	log              *log.Entry
}

func NewService(
	dataNode dataNode,
	wallet walletClient,
	account accountService,
	config *config.WhaleConfig,
) *Service {
	return &Service{
		node:   dataNode,
		wallet: wallet,

		account:          account,
		walletPubKey:     config.WalletPubKey,
		walletName:       config.WalletName,
		walletPassphrase: config.WalletPassphrase,
		log: log.WithFields(log.Fields{
			"whale": config.WalletName,
		}),
	}
}

func (w *Service) Start(ctx context.Context) error {
	w.log.Info("Starting whale service...")

	pauseCh := make(chan types.PauseSignal)

	go func() {
		for p := range pauseCh {
			w.log.Infof("Whale service paused: %v; from %s", p.Pause, p.From)
		}
	}()

	w.account.Init(w.walletPubKey, pauseCh)

	if err := w.wallet.LoginWallet(ctx, w.walletName, w.walletPassphrase); err != nil {
		return fmt.Errorf("failed to login to wallet: %s", err)
	}

	w.node.MustDialConnection(ctx)
	return nil
}

func (w *Service) TopUpAsync(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error {
	if assetID == "" {
		return fmt.Errorf("assetID is empty for bot '%s'", receiverName)
	}

	if receiverAddress == w.walletPubKey {
		return fmt.Errorf("whale and bot address cannot be the same")
	}

	go func() {
		if err := w.account.EnsureBalance(ctx, assetID, amount, "Whale"); err != nil {
			w.log.Errorf("Whale: failed to ensure enough funds: %s", err)
			return
		}

		err := w.wallet.SignTx(ctx, &v1.SubmitTransactionRequest{
			PubKey:    w.walletPubKey,
			Propagate: true,
			Command: &v1.SubmitTransactionRequest_Transfer{
				Transfer: &commV1.Transfer{
					FromAccountType: vtypes.AccountTypeGeneral,
					To:              receiverAddress,
					ToAccountType:   vtypes.AccountTypeGeneral,
					Asset:           assetID,
					Amount:          amount.String(),
					Reference:       fmt.Sprintf("Liquidity Bot '%s' Top-Up", receiverName),
					Kind:            &commV1.Transfer_OneOff{OneOff: &commV1.OneOffTransfer{}},
				},
			},
		})
		if err != nil {
			log.Errorf("Failed to top-up bot '%s': %s", receiverName, err)
		}
	}()

	return nil
}

func (w *Service) StakeAsync(ctx context.Context, receiverAddress, assetID string, amount *num.Uint) error {
	w.log.Debugf("Staking for '%s' ...", receiverAddress)

	if err := w.account.StakeAsync(ctx, receiverAddress, assetID, amount); err != nil {
		return err
	}

	return nil
}
