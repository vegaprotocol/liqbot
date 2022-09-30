package account

import (
	"context"
	"time"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
)

type accountStream interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	InitBalances(assetID string) (data.BalanceStore, error)
	WaitForStakeLinking(pubKey string) error
	WaitForDepositFinalise(ctx context.Context, walletPubKey, assetID string, amount *num.Uint, timeout time.Duration) error
}
