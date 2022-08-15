package whale

import (
	"context"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	v1 "code.vegaprotocol.io/protos/vega/wallet/v1"

	"code.vegaprotocol.io/liqbot/types/num"
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
	Stake(ctx context.Context, ownerPrivateKey, ownerAddress, vegaTokenAddress string, amount *num.Uint) (*num.Uint, error)
	Deposit(ctx context.Context, ownerPrivateKey, ownerAddress, tokenAddress string, amount *num.Uint) (*num.Uint, error)
}

type faucetClient interface {
	Mint(ctx context.Context, assetID string, amount *num.Uint) error
}

type depositStream interface {
	WaitForDepositFinalize(ctx context.Context, settlementAssetID string, amount *num.Uint, timeout time.Duration) error
}
