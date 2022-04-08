package num

import "github.com/shopspring/decimal"

// UChain ...
type UChain struct {
	z *Uint
}

// UintChain returns a Uint that supports chainable operations
// The Uint passed to the constructor is the value that will be updated, so be careful
// Things like x := NewUint(0).Add(y, z)
// x.Mul(x, foo)
// can be written as:
// x := UintChain(NewUint(0)).Add(y, z).Mul(foo).Get().
func UintChain(z *Uint) *UChain {
	return &UChain{
		z: z,
	}
}

// Get gets the result of the chained operation (the value of the wrapped uint).
func (c *UChain) Get() *Uint {
	return c.z
}

// Add is equivalent to AddSum.
func (c *UChain) Add(vals ...*Uint) *UChain {
	if len(vals) == 0 {
		return c
	}
	c.z.AddSum(vals...)
	return c
}

// Sub subtracts any numbers from the chainable value.
func (c *UChain) Sub(vals ...*Uint) *UChain {
	for _, v := range vals {
		c.z.Sub(c.z, v)
	}
	return c
}

// Mul multiplies the current value by x.
func (c *UChain) Mul(x *Uint) *UChain {
	c.z.Mul(c.z, x)
	return c
}

// Div divides the current value by x.
func (c *UChain) Div(x *Uint) *UChain {
	c.z.Div(c.z, x)
	return c
}

// DecRounding ...
type DecRounding int

// constants.
const (
	DecFloor DecRounding = iota
	DecRound
	DecCeil
)

// DChain ...
type DChain struct {
	d Decimal
}

// UintDecChain returns a chainable decimal from a given uint
// this moves the conversion stuff out from the caller.
func UintDecChain(u *Uint) *DChain {
	// @TODO once the updates to the decimal file are merged, call the coversion function from that file.
	return &DChain{
		d: decimal.NewFromBigInt(u.u.ToBig(), 0),
	}
}

// DecChain offers the same chainable interface for decimals.
func DecChain(d Decimal) *DChain {
	return &DChain{
		d: d,
	}
}

// Get returns the final value.
func (d *DChain) Get() Decimal {
	return d.d
}

// GetUint returns the decimal as a uint, returns true on overflow
// pass in type of rounding to apply
// not that the rounding does not affect the underlying decimal value
// rounding is applied to a copy only.
func (d *DChain) GetUint(round DecRounding) (*Uint, bool) {
	v := d.d
	switch round {
	case DecFloor:
		v = v.Floor()
	case DecCeil:
		v = v.Ceil()
	case DecRound:
		v = v.Round(0) // we're converting to Uint, so round to 0 places.
	}
	return UintFromBig(v.BigInt())
}

// Add adds any number of decimals together.
func (d *DChain) Add(vals ...Decimal) *DChain {
	for _, v := range vals {
		d.d = d.d.Add(v)
	}
	return d
}

// Sub subtracts any number of decimals from the chainable value.
func (d *DChain) Sub(vals ...Decimal) *DChain {
	for _, v := range vals {
		d.d = d.d.Sub(v)
	}
	return d
}

// Mul multiplies, obviously.
func (d *DChain) Mul(x Decimal) *DChain {
	d.d = d.d.Mul(x)
	return d
}

// Div divides.
func (d *DChain) Div(x Decimal) *DChain {
	d.d = d.d.Div(x)
	return d
}

// DivRound divides with a specified precision.
func (d *DChain) DivRound(x Decimal, precision int32) *DChain {
	d.d = d.d.DivRound(x, precision)
	return d
}
