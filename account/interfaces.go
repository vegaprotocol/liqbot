package account

import (
	"context"
	"time"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
)

type accountStream interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	GetBalances(ctx context.Context, assetID string) (data.BalanceStore, error)
	GetStake(ctx context.Context) (*num.Uint, error)
	WaitForStakeLinking(ctx context.Context, pubKey string) error
	WaitForTopUpToFinalise(ctx context.Context, walletPubKey, assetID string, amount *num.Uint, timeout time.Duration) error
}
