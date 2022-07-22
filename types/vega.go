package types

import "code.vegaprotocol.io/protos/vega"

// Shape is the top level definition of a liquidity shape.
type Shape struct {
	Sells []*vega.LiquidityOrder
	Buys  []*vega.LiquidityOrder
}
