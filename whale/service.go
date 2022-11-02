package whale

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
	wtypes "code.vegaprotocol.io/shared/libs/wallet/types"
	vtypes "code.vegaprotocol.io/vega/core/types"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	"code.vegaprotocol.io/vega/protos/vega"
	commV1 "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	v12 "code.vegaprotocol.io/vega/protos/vega/events/v1"
	"code.vegaprotocol.io/vega/protos/vega/wallet/v1"
	"code.vegaprotocol.io/vega/wallet/wallets"
)

type Service struct {
	node    dataNode
	wallet  walletClient
	account accountService
	faucet  faucetClient

	walletName       string
	walletPassphrase string
	walletPubKey     string
	log              *log.Entry
}

func NewService(
	dataNode dataNode,
	wallet walletClient,
	account accountService,
	faucet faucetClient,
	config *config.WhaleConfig,
) *Service {
	return &Service{
		node:   dataNode,
		wallet: wallet,
		faucet: faucet,

		account:          account,
		walletPubKey:     config.WalletPubKey,
		walletName:       config.WalletName,
		walletPassphrase: config.WalletPassphrase,
		log: log.WithFields(log.Fields{
			"component": "Whale",
			"name":      config.WalletName,
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

	if err := w.setupWallet(ctx); err != nil {
		return fmt.Errorf("failed to login to wallet: %s", err)
	}

	w.log.Info("Attempting to connect to a node...")
	w.node.MustDialConnection(ctx)
	w.log.Info("Connected to a node")

	w.account.Init(w.walletPubKey, pauseCh)
	return nil
}

func (w *Service) TopUpAsync(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) (v12.BusEventType, error) {
	w.log.Debugf("Top up for '%s' ...", receiverName)

	evt := v12.BusEventType_BUS_EVENT_TYPE_TRANSFER

	if assetID == "" {
		return evt, fmt.Errorf("assetID is empty for bot '%s'", receiverName)
	}

	if receiverAddress == w.walletPubKey {
		return evt, fmt.Errorf("whale and bot address cannot be the same")
	}

	ensureAmount := num.Zero().Mul(amount, num.NewUint(30))

	response, err := w.node.AssetByID(ctx, &dataapipb.AssetByIDRequest{
		Id: assetID,
	})
	if err != nil {
		return v12.BusEventType_BUS_EVENT_TYPE_UNSPECIFIED, fmt.Errorf("failed to get asset by id: %w", err)
	}

	if builtin := response.Asset.Details.GetBuiltinAsset(); builtin != nil {
		if err := w.depositBuiltin(ctx, assetID, receiverAddress, ensureAmount, builtin); err != nil {
			return v12.BusEventType_BUS_EVENT_TYPE_UNSPECIFIED, errors.Wrap(err, "failed to deposit builtin")
		}
		return v12.BusEventType_BUS_EVENT_TYPE_UNSPECIFIED, nil
	}

	go func() {
		w.log.WithFields(
			log.Fields{
				"receiverName":   receiverName,
				"receiverPubKey": receiverAddress,
				"assetID":        assetID,
				"amount":         amount.String(),
			}).Debugf("Ensuring account has enough balance...")

		// TODO: retry
		if err := w.account.EnsureBalance(ctx, assetID, types.General, ensureAmount, "Whale"); err != nil {
			w.log.Errorf("Whale: failed to ensure enough funds: %s", err)
			return
		}

		w.log.WithFields(
			log.Fields{
				"receiverName":   receiverName,
				"receiverPubKey": receiverAddress,
				"assetID":        assetID,
				"amount":         amount.String(),
			}).Debugf("Whale balance ensured, sending funds...")

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
			w.log.Errorf("Failed to top-up bot '%s': %s", receiverName, err)
			return
		}

		w.log.WithFields(
			log.Fields{
				"receiverName":   receiverName,
				"receiverPubKey": receiverAddress,
				"assetID":        assetID,
				"amount":         amount.String(),
			}).Debugf("Top-up sent")
	}()

	return evt, nil
}

func (w *Service) setupWallet(ctx context.Context) error {
	if err := w.wallet.LoginWallet(ctx, w.walletName, w.walletPassphrase); err != nil {
		if strings.Contains(err.Error(), wallets.ErrWalletDoesNotExists.Error()) {
			mnemonic, err := w.wallet.CreateWallet(ctx, w.walletName, w.walletPassphrase)
			if err != nil {
				return fmt.Errorf("failed to create wallet: %w", err)
			}
			w.log.WithFields(log.Fields{"mnemonic": mnemonic}).Info("Created and logged into wallet")
		} else {
			return fmt.Errorf("failed to log into wallet: %w", err)
		}
	}

	w.log.Info("Logged into wallet")

	publicKeys, err := w.wallet.ListPublicKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to list public keys: %w", err)
	}

	if len(publicKeys) == 0 {
		key, err := w.wallet.GenerateKeyPair(ctx, w.walletPassphrase, []wtypes.Meta{})
		if err != nil {
			return fmt.Errorf("failed to generate keypair: %w", err)
		}
		w.walletPubKey = key.Pub
		w.log.WithFields(log.Fields{"pubKey": w.walletPubKey}).Debug("Created keypair")
	} else {
		w.walletPubKey = publicKeys[0]
		w.log.WithFields(log.Fields{"pubKey": w.walletPubKey}).Debug("Using existing keypair")
	}

	w.log = w.log.WithFields(log.Fields{"pubkey": w.walletPubKey})

	return nil
}

func (w *Service) depositBuiltin(ctx context.Context, assetID, pubKey string, amount *num.Uint, builtin *vega.BuiltinAsset) error {
	maxFaucet, err := num.ConvertUint256(builtin.MaxFaucetAmountMint)
	if err != nil {
		return fmt.Errorf("failed to convert max faucet amount: %w", err)
	}

	if maxFaucet.GT(amount) {
		if ok, err := w.faucet.Mint(ctx, maxFaucet.String(), assetID, pubKey); err != nil {
			return fmt.Errorf("failed to mint: %w", err)
		} else if !ok {
			return fmt.Errorf("failed to mint")
		}
		return nil
	}

	times := int(new(num.Uint).Div(amount, maxFaucet).Uint64() + 1)
	totalMinted := new(num.Uint)

	// TODO: limit the time here!

	for i := 0; i < times; i++ {
		if ok, err := w.faucet.Mint(ctx, maxFaucet.String(), assetID, pubKey); err != nil {
			return fmt.Errorf("failed to mint: %w", err)
		} else if !ok {
			return fmt.Errorf("failed to mint")
		}

		totalMinted.Add(totalMinted, maxFaucet)

		time.Sleep(2 * time.Second) // TODO: configure
		w.log.Infof("Minted %s out of %s for %s", totalMinted, amount, assetID)
	}

	return nil
}

func (w *Service) StakeAsync(ctx context.Context, receiverAddress, assetID string, amount *num.Uint) error {
	w.log.Debugf("Staking for '%s' ...", receiverAddress)

	if err := w.account.StakeAsync(ctx, receiverAddress, assetID, amount); err != nil {
		return err
	}

	return nil
}
