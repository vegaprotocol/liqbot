package whale

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	commV1 "code.vegaprotocol.io/protos/vega/commands/v1"
	"code.vegaprotocol.io/protos/vega/wallet/v1"
	vtypes "code.vegaprotocol.io/vega/types"
	"github.com/ethereum/go-ethereum/crypto"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/util"
)

type Service struct {
	walletName       string
	walletPassphrase string
	walletPubKey     string
	node             dataNode
	wallet           walletClient
	erc20            erc20Service
	faucet           faucetClient
	depositStream    depositStream
	ownerPrivateKeys map[string]string
	callTimeoutMills time.Duration
}

func NewService(
	dataNode dataNode,
	wallet walletClient,
	erc20 erc20Service,
	faucet faucetClient,
	deposits depositStream,
	config *config.WhaleConfig,
) *Service {
	return &Service{
		node:             dataNode,
		wallet:           wallet,
		erc20:            erc20,
		faucet:           faucet,
		depositStream:    deposits,
		walletPubKey:     config.WalletPubKey,
		walletName:       config.WalletName,
		walletPassphrase: config.WalletPassphrase,
		ownerPrivateKeys: config.OwnerPrivateKeys,
		callTimeoutMills: time.Duration(config.SyncTimeoutSec) * time.Second,
	}
}

func (s *Service) Start(ctx context.Context) error {
	if err := s.wallet.LoginWallet(ctx, s.walletName, s.walletPassphrase); err != nil {
		return fmt.Errorf("failed to login to wallet: %w", err)
	}

	s.node.MustDialConnection(ctx)
	return nil
}

func (s *Service) TopUp(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error {
	if receiverAddress == s.walletPubKey {
		return fmt.Errorf("the sender and receiver address cannot be the same")
	}

	if err := s.ensureWhaleBalance(ctx, assetID, amount); err != nil {
		return fmt.Errorf("failed to ensure enough funds: %w", err)
	}

	// TODO: is Vega staked token supposed to be transferred differently?

	err := s.wallet.SignTx(ctx, &v1.SubmitTransactionRequest{
		PubKey:    s.walletPubKey,
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
		return fmt.Errorf("failed to top-up bot '%s': %w", receiverName, err)
	}

	if err = s.depositStream.WaitForDepositFinalize(ctx, assetID, amount, s.callTimeoutMills); err != nil {
		return fmt.Errorf("failed to finalize deposit: %w", err)
	}

	return nil
}

func (s *Service) ensureWhaleBalance(ctx context.Context, assetID string, amount *num.Uint) error {
	balance, err := s.getBalance(assetID)
	if err != nil {
		return fmt.Errorf("failed to check for sufficient funds: %w", err)
	}

	if balance.GT(amount.Int()) {
		return nil
	}

	toDeposit := amount.Mul(amount, num.NewUint(1000)) // deposit more than needed

	if err = s.deposit(ctx, assetID, toDeposit); err != nil {
		return fmt.Errorf("failed to deposit: %w", err)
	}

	return nil
}

func (s *Service) getBalance(assetID string) (*num.Int, error) {
	// cache the balance
	response, err := s.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: s.walletPubKey,
		Asset:   assetID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	balance := new(num.Uint)

	for _, account := range response.Accounts {
		if account.Type == vtypes.AccountTypeGeneral {
			balance, err = util.ConvertUint256(account.Balance)
			if err != nil {
				return nil, fmt.Errorf("failed to convert account balance: %w", err)
			}
		}
	}

	return balance.Int(), nil
}

func (s *Service) deposit(ctx context.Context, assetID string, amount *num.Uint) error {
	response, err := s.node.AssetByID(&dataapipb.AssetByIDRequest{
		Id: assetID,
	})
	if err != nil {
		return fmt.Errorf("failed to get asset id: %w", err)
	}

	if erc20 := response.Asset.Details.GetErc20(); erc20 != nil {
		return s.depositERC20(ctx, response.Asset, amount)
	} else if builtin := response.Asset.Details.GetBuiltinAsset(); builtin != nil {
		maxFaucet, err := util.ConvertUint256(builtin.MaxFaucetAmountMint)
		if err != nil {
			return fmt.Errorf("failed to convert max faucet amount: %w", err)
		}
		return s.depositBuiltin(ctx, assetID, amount, maxFaucet)
	}

	return fmt.Errorf("unsupported asset type")
}

func (s *Service) depositERC20(ctx context.Context, asset *vega.Asset, amount *num.Uint) error {
	ownerKey, err := s.getOwnerKeyForAsset(asset.Id)
	if err != nil {
		return fmt.Errorf("failed to get owner key: %w", err)
	}

	contractAddress := asset.Details.GetErc20().ContractAddress
	added := new(num.Uint)

	if asset.Details.Symbol == "VEGA" {
		added, err = s.erc20.Stake(ctx, ownerKey.privateKey, ownerKey.address, contractAddress, amount)
	} else {
		added, err = s.erc20.Deposit(ctx, ownerKey.privateKey, ownerKey.address, contractAddress, amount)
	}
	if err != nil {
		return fmt.Errorf("failed to add erc20 token: %w", err)
	}

	// TODO: check units
	if added.Int().LT(amount.Int()) {
		// TODO: how to proceed?
		return fmt.Errorf("deposited less than requested amount")
	}

	return nil
}

func (s *Service) depositBuiltin(ctx context.Context, assetID string, amount, maxFaucet *num.Uint) error {
	times := int(new(num.Uint).Div(amount, maxFaucet).Uint64() + 1)

	for i := 0; i < times; i++ {
		if err := s.faucet.Mint(ctx, assetID, maxFaucet); err != nil {
			return fmt.Errorf("failed to deposit: %w", err)
		}
		time.Sleep(2 * time.Second) // TODO: configure
	}
	return nil
}

type key struct {
	privateKey string
	address    string
}

func (s *Service) getOwnerKeyForAsset(assetID string) (*key, error) {
	ownerPrivateKey, ok := s.ownerPrivateKeys[assetID]
	if !ok {
		return nil, fmt.Errorf("owner private key not configured for asset '%s'", assetID)
	}

	address, err := addressFromPrivateKey(ownerPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get address from private key: %w", err)
	}

	return &key{
		privateKey: ownerPrivateKey,
		address:    address,
	}, nil
}

func addressFromPrivateKey(privateKey string) (string, error) {
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert owner private key hash into ECDSA: %w", err)
	}

	publicKeyECDSA, ok := key.Public().(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return address, nil
}

func (s *Service) slackDan(_ context.Context, address, assetID string, amount *num.Uint) error {
	// TODO: slack @Dan
	log.Printf("slack @Dan: %s %s %s", address, assetID, amount)
	return nil
}
