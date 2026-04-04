package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatingMode_String(t *testing.T) {
	tests := []struct {
		mode OperatingMode
		want string
	}{
		{ModeDeterministic, "deterministic"},
		{ModeStateful, "stateful"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.mode.String(), "OperatingMode.String() for %v", tt.mode)
	}
}

func TestOperatingMode_IsValid(t *testing.T) {
	assert.True(t, ModeDeterministic.IsValid())
	assert.True(t, ModeStateful.IsValid())
	assert.False(t, OperatingMode("bogus").IsValid())
	assert.False(t, OperatingMode("").IsValid())
	assert.False(t, OperatingMode("Deterministic").IsValid(), "case-sensitive")
}

func TestParseOperatingMode(t *testing.T) {
	mode, err := ParseOperatingMode("deterministic")
	require.NoError(t, err)
	assert.Equal(t, ModeDeterministic, mode)

	mode, err = ParseOperatingMode("stateful")
	require.NoError(t, err)
	assert.Equal(t, ModeStateful, mode)

	_, err = ParseOperatingMode("bogus")
	require.ErrorIs(t, err, ErrInvalidOperatingMode)

	_, err = ParseOperatingMode("")
	require.ErrorIs(t, err, ErrInvalidOperatingMode)
}
