package config

import (
	"fmt"

	"code.vegaprotocol.io/liqbot/types/num"
)

// ConfigUint is for storing a num.Uint as a string in a config file
type ConfigUint struct {
	u num.Uint
}

func (u *ConfigUint) Get() *num.Uint {
	return u.u.Clone()
}

// UnmarshalText converts a string to a nun.Uint
func (u *ConfigUint) UnmarshalText(text []byte) error {
	value, err_or_overflow := num.UintFromString(string(text), 10)
	if err_or_overflow {
		return fmt.Errorf("failed to unmarshal uint256: error or overflow \"%s\"", string(text))
	}
	u.u.Set(value)
	return nil
}

// MarshalText converts a ConfigUint to a string
func (u ConfigUint) MarshalText() ([]byte, error) {
	return []byte(u.u.String()), nil
}
