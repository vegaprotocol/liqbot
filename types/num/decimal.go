package num

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// Decimal is a shopspring.Decimal.
type Decimal = decimal.Decimal

var (
	dzero      = decimal.Zero
	maxDecimal = decimal.NewFromBigInt(maxU256, 0)
)

// DecimalZero ...
func DecimalZero() Decimal {
	return dzero
}

// MaxDecimal ...
func MaxDecimal() Decimal {
	return maxDecimal
}

// NewDecimalFromFloat ...
func NewDecimalFromFloat(f float64) Decimal {
	return decimal.NewFromFloat(f)
}

// NewDecimalFromBigInt ...
func NewDecimalFromBigInt(value *big.Int, exp int32) Decimal {
	return decimal.NewFromBigInt(value, exp)
}

// DecimalFromUint ...
func DecimalFromUint(u *Uint) Decimal {
	return decimal.NewFromUint(&u.u)
}

// DecimalFromInt64 ...
func DecimalFromInt64(i int64) Decimal {
	return decimal.NewFromInt(i)
}

// DecimalFromFloat ...
func DecimalFromFloat(v float64) Decimal {
	return decimal.NewFromFloat(v)
}

// DecimalFromString ...
func DecimalFromString(s string) (Decimal, error) {
	return decimal.NewFromString(s)
}

// MaxD ...
func MaxD(a, b Decimal) Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}

// MinD ...
func MinD(a, b Decimal) Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}
