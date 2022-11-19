package whale

import (
	"context"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
	wtypes "code.vegaprotocol.io/shared/libs/wallet/types"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	v1 "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type dataNode interface {
	AssetByID(ctx context.Context, req *dataapipb.GetAssetRequest) (*vega.Asset, error)
	MustDialConnection(ctx context.Context)
}

type walletClient interface {
	CreateWallet(ctx context.Context, name, passphrase string) (string, error)
	ListPublicKeys(ctx context.Context) ([]string, error)
	GenerateKeyPair(ctx context.Context, passphrase string, meta []wtypes.Meta) (*wtypes.Key, error)
	LoginWallet(ctx context.Context, name, passphrase string) error
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) error
}

type erc20Service interface {
	Stake(ctx context.Context, ownerPrivateKey, ownerAddress, vegaTokenAddress, vegaPubKey string, amount *num.Uint) (*num.Uint, error)
	Deposit(ctx context.Context, ownerPrivateKey, ownerAddress, tokenAddress, vegaPubKey string, amount *num.Uint) (*num.Uint, error)
}

type faucetClient interface {
	Mint(ctx context.Context, amount string, asset, party string) (bool, error)
}

type accountService interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	EnsureBalance(ctx context.Context, assetID string, balanceFn func(types.Balance) *num.Uint, targetAmount *num.Uint, from string) error
	EnsureStake(ctx context.Context, receiverName, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error
	StakeAsync(ctx context.Context, receiverPubKey, assetID string, amount *num.Uint) error
}
