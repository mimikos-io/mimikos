package generator

import (
	"math/big"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// --- Shared test helpers ---

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// newRat creates a *big.Rat from an int64.
func newRat(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

// newRatF creates a *big.Rat from a float64.
func newRatF(f float64) *big.Rat { return new(big.Rat).SetFloat64(f) }

// newTypes creates a *jsonschema.Types from type name strings.
func newTypes(types ...string) *jsonschema.Types {
	var tt jsonschema.Types
	for _, t := range types {
		tt.Add(t)
	}

	return &tt
}

// newFormat creates a *jsonschema.Format with the given name.
func newFormat(name string) *jsonschema.Format {
	return &jsonschema.Format{Name: name}
}

// newEnum creates a *jsonschema.Enum with the given values.
func newEnum(values ...any) *jsonschema.Enum {
	return &jsonschema.Enum{Values: values}
}

// objectSchema creates an object schema with the given properties and required fields.
func objectSchema(props map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	s := &jsonschema.Schema{
		Types:      newTypes("object"),
		Properties: props,
	}

	if len(required) > 0 {
		s.Required = required
	}

	return s
}
