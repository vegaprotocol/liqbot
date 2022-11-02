package account

import (
	"context"
	"time"

	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/num"
	v1 "code.vegaprotocol.io/vega/protos/vega/events/v1"
)

type accountStream interface {
	Init(pubKey string, pauseCh chan types.PauseSignal)
	GetBalances(ctx context.Context, assetID string) (data.BalanceStore, error)
	GetStake(ctx context.Context) (*num.Uint, error)
	WaitForStakeLinking(ctx context.Context, pubKey string) error
	WaitForTopUpToFinalise(ctx context.Context, evtType v1.BusEventType, walletPubKey, assetID string, amount *num.Uint, timeout time.Duration) error
}
