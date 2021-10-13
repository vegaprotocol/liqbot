module code.vegaprotocol.io/liqbot

go 1.16

require (
	code.vegaprotocol.io/go-wallet v0.9.0-pre4
	code.vegaprotocol.io/priceproxy v0.0.2
	code.vegaprotocol.io/protos v0.44.0 // indirect
	code.vegaprotocol.io/shared v0.0.0-20210920084547-4d2225c561d6
	github.com/golang/mock v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/holiman/uint256 v1.2.0 // indirect
	github.com/jinzhu/configor v1.2.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/pkg/errors v0.9.1
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/vegaprotocol/api v0.42.0-pre4
	go.uber.org/zap v1.18.1 // indirect
	gonum.org/v1/gonum v0.9.1
	google.golang.org/grpc v1.41.0
	gopkg.in/yaml.v2 v2.3.0 // indirect
)

replace github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.2.1-0.20210705145732-aaa563729a0a
