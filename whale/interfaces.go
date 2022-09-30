package whale

import (
	"context"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	v1 "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
)

type dataNode interface {
	AssetByID(req *dataapipb.AssetByIDRequest) (*dataapipb.AssetByIDResponse, error)
	PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error)
	MustDialConnection(ctx context.Context)
}

type walletClient interface {
	LoginWallet(ctx context.Context, name, passphrase string) error
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) error
}

type erc20Service interface {
	StakeToAddress(ctx context.Context, ownerPrivateKey, ownerAddress, vegaTokenAddress, vegaPubKey string, amount *num.Uint) (*num.Uint, error)
	Deposit(ctx context.Context, ownerPrivateKey, ownerAddress, tokenAddress string, amount *num.Uint) (*num.Uint, error)
}

type faucetClient interface {
	Mint(ctx context.Context, assetID string, amount *num.Uint) error
}

type accountService interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	EnsureBalance(ctx context.Context, assetID string, targetAmount *num.Uint, from string) error
	EnsureStake(ctx context.Context, receiverPubKey, assetID string, targetAmount *num.Uint, from string) error
	StakeAsync(ctx context.Context, receiverPubKey, assetID string, amount *num.Uint) error
}
