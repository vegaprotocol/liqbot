module code.vegaprotocol.io/liqbot

go 1.19

require (
	code.vegaprotocol.io/priceproxy v0.0.2
	code.vegaprotocol.io/shared v0.0.0-20220614080106-5c97205b0d92
	code.vegaprotocol.io/vega v0.72.2-0.20230815101118-83406455de11
	github.com/golang/mock v1.6.1-0.20220512030613-73266f9366fc
	github.com/hashicorp/go-multierror v1.1.1
	github.com/holiman/uint256 v1.2.2-0.20230321075855-87b91420868c
	github.com/jinzhu/configor v1.2.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/shopspring/decimal v1.3.1
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.2
	golang.org/x/exp v0.0.0-20230321023759-10a507213a29
	gonum.org/v1/gonum v0.12.0
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f
	google.golang.org/grpc v1.53.0
	google.golang.org/protobuf v1.30.0
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/adrg/xdg v0.4.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/ethereum/go-ethereum v1.11.6 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.9.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/oasisprotocol/curve25519-voi v0.0.0-20220317090546-adb2f9614b17 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/vegaprotocol/go-slip10 v0.1.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/fergusstrange/embedded-postgres => github.com/vegaprotocol/embedded-postgres v1.13.1-0.20221123183204-2e7a2feee5bb
	github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.3.1-uint256
)

replace github.com/tendermint/tendermint => github.com/informalsystems/tendermint v0.34.24
