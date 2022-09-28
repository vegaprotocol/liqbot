package whale

import (
	"context"
	"testing"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/token"
	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/wallet"
)

func TestService_TopUp(t *testing.T) {
	wKey := "8528c773a4b62d7609182cb0eb66dd1545173332b569397b8b1f7c1901f90932"
	wName := "your_wallet_name"
	wPassphrase := "super-secret"

	dn := node.NewDataNode([]string{"localhost:3027"}, 10000)
	wc := wallet.NewClient("http://localhost:1789")
	es, err := token.NewService(&config.TokenConfig{
		EthereumAPIAddress:   "ws://127.0.0.1:8545",
		Erc20BridgeAddress:   "0x9708FF7510D4A7B9541e1699d15b53Ecb1AFDc54",
		StakingBridgeAddress: "0x9135f5afd6F055e731bca2348429482eE614CFfA",
		SyncTimeoutSec:       100,
	}, wKey)
	if err != nil {
		t.Errorf("could not create erc20 service = %s", err)
		return
	}

	fc := token.NewFaucetService("http://localhost:1790", wKey)
	ds := data.NewDepositStream(dn, wKey)

	dk := map[string]string{
		"993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede": "a37f4c2a678aefb5037bf415a826df1540b330b7e471aa54184877ba901b9ef0",
		"b4f2726571fbe8e33b442dc92ed2d7f0d810e21835b7371a7915a365f07ccd9b": "a37f4c2a678aefb5037bf415a826df1540b330b7e471aa54184877ba901b9ef0",
	}

	conf := &config.WhaleConfig{
		WalletPubKey:     wKey,
		WalletName:       wName,
		WalletPassphrase: wPassphrase,
		OwnerPrivateKeys: dk,
		SyncTimeoutSec:   100,
	}

	s := NewService(dn, wc, es, fc, ds, conf)

	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Errorf("Start() error = %s", err)
		return
	}

	key := "8528c773a4b62d7609182cb0eb66dd1545173332b569397b8b1f7c1901f90932"
	asset := "b4f2726571fbe8e33b442dc92ed2d7f0d810e21835b7371a7915a365f07ccd9b"
	// asset := "993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede"
	if err := s.TopUp(ctx, "some bot", key, asset, num.NewUint(1100000000000000000)); err != nil {
		t.Errorf("TopUp() error = %s", err)
	}
}
