package model

import (
	"errors"
	"fmt"
)

// OperatingMode controls how the mock server generates responses.
// In deterministic mode, responses are derived from request fingerprints.
// In stateful mode, responses reflect accumulated state from prior requests.
type OperatingMode string

const (
	// ModeDeterministic generates responses from request fingerprints.
	// Same request always produces the same response.
	ModeDeterministic OperatingMode = "deterministic"

	// ModeStateful maintains resource state across requests.
	// POST creates resources, GET retrieves them, DELETE removes them.
	ModeStateful OperatingMode = "stateful"
)

// ErrInvalidOperatingMode is returned when parsing an unknown operating mode string.
var ErrInvalidOperatingMode = errors.New("invalid operating mode")

//nolint:gochecknoglobals // immutable lookup table for mode validation
var validModes = map[OperatingMode]struct{}{
	ModeDeterministic: {},
	ModeStateful:      {},
}

// String returns the string representation of the operating mode.
func (m OperatingMode) String() string {
	return string(m)
}

// IsValid returns true if the operating mode is one of the defined constants.
func (m OperatingMode) IsValid() bool {
	_, ok := validModes[m]

	return ok
}

// ParseOperatingMode converts a string to an OperatingMode, returning
// ErrInvalidOperatingMode if the string is not recognized.
func ParseOperatingMode(s string) (OperatingMode, error) {
	m := OperatingMode(s)
	if !m.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidOperatingMode, s)
	}

	return m, nil
}
