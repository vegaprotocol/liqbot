module code.vegaprotocol.io/liqbot

go 1.16

require (
	code.vegaprotocol.io/priceproxy v0.0.2
	code.vegaprotocol.io/protos v0.49.3-0.20220318134118-b50784501b9d
	code.vegaprotocol.io/vegawallet v0.13.2-0.20220318145033-e10ba6ba6ad7
	github.com/golang/mock v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/holiman/uint256 v1.2.0
	github.com/jinzhu/configor v1.2.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kr/text v0.2.0 // indirect
	github.com/shopspring/decimal v1.3.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	gonum.org/v1/gonum v0.9.1
	google.golang.org/genproto v0.0.0-20220314164441-57ef72a4c106
	google.golang.org/grpc v1.45.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.2.1-0.20210705145732-aaa563729a0a
