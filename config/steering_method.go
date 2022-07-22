package config

import "fmt"

// SteeringMethod is an enum for all the possible price calculations methods for price steering.
type SteeringMethod int

const (
	// NotSet for when we cannot parse the input string.
	NotSet SteeringMethod = iota
	// DiscreteThreeLevel uses the discrete three level method.
	DiscreteThreeLevel
	// CoinAndBinomial uses the coin and binomial method.
	CoinAndBinomial
)

const (
	DiscreteThreeLevelString = "discreteThreeLevel"
	CoinAndBinomialString    = "coinAndBinomial"
)

func stringToSteeringMethod(method string) (SteeringMethod, error) {
	switch method {
	case DiscreteThreeLevelString:
		return DiscreteThreeLevel, nil
	case CoinAndBinomialString:
		return CoinAndBinomial, nil
	}
	return NotSet, fmt.Errorf("steering method unknown:%s", method)
}

// Get returns the underlying string.
func (s *SteeringMethod) Get() string {
	return s.String()
}

// Get returns the underlying string
func (s SteeringMethod) String() string {
	switch s {
	case DiscreteThreeLevel:
		return DiscreteThreeLevelString
	case CoinAndBinomial:
		return CoinAndBinomialString
	}

	return ""
}

// UnmarshalText converts a string to a SteeringMethod.
func (s *SteeringMethod) UnmarshalText(text []byte) error {
	value, err := stringToSteeringMethod(string(text))
	if err != nil {
		return fmt.Errorf("failed to unmarshal steering method: %w", err)
	}

	*s = value
	return nil
}

// MarshalText converts a SteeringMethod to a string.
func (s SteeringMethod) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}
