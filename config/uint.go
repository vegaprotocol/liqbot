package config

import (
	"fmt"

	"code.vegaprotocol.io/liqbot/types/num"
)

// Uint is for storing a num.Uint as a string in a config file.
type Uint struct {
	u num.Uint
}

// Get returns the underlying num.Uint.
func (u *Uint) Get() *num.Uint {
	return u.u.Clone()
}

// UnmarshalText converts a string to a nun.Uint.
func (u *Uint) UnmarshalText(text []byte) error {
	value, errOrOverflow := num.UintFromString(string(text), 10)
	if errOrOverflow {
		return fmt.Errorf("failed to unmarshal uint256: error or overflow \"%s\"", string(text))
	}
	u.u.Set(value)
	return nil
}

// MarshalText converts a Uint to a string.
func (u Uint) MarshalText() ([]byte, error) {
	return []byte(u.u.String()), nil
}
