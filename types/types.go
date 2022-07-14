package types

import (
	"code.vegaprotocol.io/protos/vega"

	"code.vegaprotocol.io/liqbot/types/num"
)

type MarketData struct {
	PriceStaticMid *num.Uint
	PriceMark      *num.Uint
	TradingMode    vega.Market_TradingMode
}

type Balance struct {
	General *num.Uint
	Margin  *num.Uint
	Bond    *num.Uint
}
