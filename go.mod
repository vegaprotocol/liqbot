module code.vegaprotocol.io/liqbot

go 1.16

require (
	code.vegaprotocol.io/go-wallet v0.9.0-pre4.0.20211007082923-29e4a2c108a1
	code.vegaprotocol.io/priceproxy v0.0.2
	code.vegaprotocol.io/protos v0.44.0
	code.vegaprotocol.io/vega v0.44.2
	github.com/golang/mock v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/holiman/uint256 v1.2.0
	github.com/jinzhu/configor v1.2.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/shopspring/decimal v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/vegaprotocol/api v0.42.0-pre4
	gonum.org/v1/gonum v0.9.1
	google.golang.org/genproto v0.0.0-20211008145708-270636b82663
	google.golang.org/grpc v1.41.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.2.1-0.20210705145732-aaa563729a0a
