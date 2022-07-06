module code.vegaprotocol.io/liqbot

go 1.16

require (
	code.vegaprotocol.io/priceproxy v0.0.2
	code.vegaprotocol.io/protos v0.50.4-0.20220428192224-eedc5d5fe8c5
	code.vegaprotocol.io/shared v0.0.0-20220704150014-7c22d12ccb72
	code.vegaprotocol.io/vegawallet v0.14.2-pre1.0.20220429133813-c772b6aa7cc1
	github.com/ethereum/go-ethereum v1.10.20
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/hashicorp/go-multierror v1.1.1
	github.com/holiman/uint256 v1.2.0
	github.com/jinzhu/configor v1.2.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/shopspring/decimal v1.3.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.2
	gonum.org/v1/gonum v0.9.1
	google.golang.org/genproto v0.0.0-20220405205423-9d709892a2bf
	google.golang.org/grpc v1.45.0
	google.golang.org/protobuf v1.28.0
)

replace github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.2.1-0.20210705145732-aaa563729a0a
