package types

import (
	"code.vegaprotocol.io/protos/vega"

	"code.vegaprotocol.io/liqbot/types/num"
)

type MarketData struct {
	StaticMidPrice *num.Uint
	MarkPrice      *num.Uint
	TradingMode    vega.Market_TradingMode
}

type Balance struct {
	General *num.Uint
	Margin  *num.Uint
	Bond    *num.Uint
}

func (b Balance) Total() *num.Uint {
	return num.Sum(b.General, b.Margin, b.Bond)
}
