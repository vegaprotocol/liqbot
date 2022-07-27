package util

import (
	"errors"

	"code.vegaprotocol.io/liqbot/types/num"
)

func ConvertUint256(valueStr string) (*num.Uint, error) {
	value, overflowed := num.UintFromString(valueStr, 10)
	if overflowed {
		return nil, errors.New("invalid uint256, needs to be base 10")
	}

	return value, nil
}
