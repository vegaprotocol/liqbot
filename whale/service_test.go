package whale

import (
	"context"
	"net/url"
	"testing"

	"code.vegaprotocol.io/liqbot/account"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/erc20"
	sconfig "code.vegaprotocol.io/shared/libs/erc20/config"
	"code.vegaprotocol.io/shared/libs/faucet"
	"code.vegaprotocol.io/shared/libs/node"
	"code.vegaprotocol.io/shared/libs/num"
	"code.vegaprotocol.io/shared/libs/wallet"
)

func TestService_TopUp(t *testing.T) {
	t.Skip()

	wKey := "6baf7809b6143d4be4a5b641fcef29947aeaa1ab3805c5442de8a31a3449078f"
	wName := "w00"
	wPassphrase := "123"

	dn := node.NewDataNode([]string{"localhost:3027"}, 10000)
	wc := wallet.NewClient("http://localhost:1789")
	es, err := erc20.NewService((*sconfig.TokenConfig)(&config.TokenConfig{
		EthereumAPIAddress:   "ws://127.0.0.1:8545",
		Erc20BridgeAddress:   "0x9708FF7510D4A7B9541e1699d15b53Ecb1AFDc54",
		StakingBridgeAddress: "0x9135f5afd6F055e731bca2348429482eE614CFfA",
		SyncTimeoutSec:       100,
	}), wKey)
	if err != nil {
		t.Errorf("could not create erc20 service = %s", err)
		return
	}

	dk := map[string]string{
		"993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede": "a37f4c2a678aefb5037bf415a826df1540b330b7e471aa54184877ba901b9ef0",
	}

	conf := &config.WhaleConfig{
		WalletPubKey:     wKey,
		WalletName:       wName,
		WalletPassphrase: wPassphrase,
		OwnerPrivateKeys: dk,
		SyncTimeoutSec:   100,
	}

	addr, err := url.Parse("http://localhost:1790")
	if err != nil {
		t.Errorf("could not parse url = %s", err)
		return
	}

	fc := faucet.New(*addr)

	cp := NewProvider(dn, es, fc, conf)

	ds := data.NewAccountStream("test", dn)
	as := account.NewAccountService("test", "asset", ds, cp)

	s := NewService(dn, wc, as, fc, conf)

	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Errorf("Start() error = %s", err)
		return
	}

	key := "69f684c78deefa27fd216ba771e4ca08085dea8e2b1dafd2c62352dda1e89073"
	asset := "993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede"

	amount := num.NewUint(988939143512)

	_, _ = s.TopUpAsync(ctx, "some bot", key, asset, amount)

	_ = s.account.EnsureBalance(ctx, asset, types.General, amount, "test")
}

func TestService_slackDan(t *testing.T) {
	t.Skip()

	type fields struct {
		walletPubKey string
	}
	type args struct {
		ctx     context.Context
		assetID string
		amount  *num.Uint
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "slack",
			fields: fields{
				walletPubKey: "6baf7809b6143d4be4a5b641fcef29947aeaa1ab3805c5442de8a31a3449078f",
			},
			args: args{
				ctx:     context.Background(),
				assetID: "993ed98f4f770d91a796faab1738551193ba45c62341d20597df70fea6704ede",
				amount:  num.NewUint(988939143512),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Provider{
				walletPubKey: tt.fields.walletPubKey,
			}
			if _, err := s.slackDan(tt.args.ctx, tt.args.assetID, tt.args.amount); (err != nil) != tt.wantErr {
				t.Errorf("slackDan() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
