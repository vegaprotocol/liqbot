package token

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"code.vegaprotocol.io/liqbot/config"
)

func TestService_mintToken(t *testing.T) {
	t.Skip()

	conf := &config.TokenConfig{
		EthereumAPIAddress: "wss://ropsten.infura.io/ws/v3/0b0e1795edae41f59f4c99d29ba0ae8e",
	}
	pubKey := "0x0"
	privKey := ""
	tokenAddr := common.HexToAddress("0x3773A5c7aFF77e014cBF067dd31801b4C6dc4136")
	toAddress := common.HexToAddress("0xb89A165EA8b619c14312dB316BaAa80D2a98B493")

	s, err := NewService(conf, pubKey)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*150)
	defer cancel()

	erc20Token, err := s.client.NewBaseTokenSession(ctx, privKey, tokenAddr, s.syncTimeout)
	if err != nil {
		t.Fatal(err)
	}

	amount := big.NewInt(7675000000000)
	minted, err := s.mintToken(ctx, erc20Token, toAddress, amount)
	if err != nil {
		t.Errorf("mintToken() error = %s", err)
	}

	if minted.Cmp(amount) != 0 {
		t.Errorf("mintToken() = %s, want %s", minted, amount)
	}
}
