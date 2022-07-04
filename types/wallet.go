package types

import (
	v12 "code.vegaprotocol.io/protos/vega/commands/v1"
	"code.vegaprotocol.io/protos/vega/wallet/v1"
)

type Meta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Key struct {
	Pub     string `json:"pub"`
	Algo    string `json:"algo"`
	Tainted bool   `json:"tainted"`
	Meta    []Meta `json:"meta"`
}

type WalletClient interface {
	CreateWallet(name, passphrase string) error
	LoginWallet(name, passphrase string) error
	ListPublicKeys() ([]string, error)
	GenerateKeyPair(passphrase string, meta []Meta) (*Key, error)
	SignTx(req *v1.SubmitTransactionRequest) (*v12.Transaction, error)
}
