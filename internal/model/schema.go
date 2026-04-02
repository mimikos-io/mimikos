package model

import "github.com/santhosh-tekuri/jsonschema/v6"

// CompiledSchema wraps a pre-compiled JSON Schema validator for
// request/response validation. Produced by the Schema Compiler
// from OpenAPI schemas parsed by the Spec Parser.
type CompiledSchema struct {
	// Name is a human-readable identifier for the schema (e.g., "Pet", "Error").
	Name string

	// IsCircular indicates that this schema contains circular references
	// and must be bounded during data generation.
	IsCircular bool

	// Schema is the compiled JSON Schema validator from santhosh-tekuri/jsonschema.
	// Used for validating JSON documents against the schema.
	Schema *jsonschema.Schema
}

// Validate validates a JSON document against the compiled schema.
// The value should be the result of json.Unmarshal into any (map[string]any,
// []any, string, float64, bool, or nil).
// Nil-safe: returns nil if the receiver or the underlying schema is nil.
func (cs *CompiledSchema) Validate(value any) error {
	if cs == nil || cs.Schema == nil {
		return nil
	}

	return cs.Schema.Validate(value)
}
