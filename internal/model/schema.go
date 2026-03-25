package model

// CompiledSchema is a placeholder wrapper for a pre-compiled JSON schema.
// In Session 10 (Task 6.4), this will wrap santhosh-tekuri/jsonschema
// compiled schemas for request/response validation.
//
// For now, it serves as the type contract that other packages depend on,
// allowing the parser and classifier to reference schema types without
// the full compilation implementation.
type CompiledSchema struct {
	// Name is a human-readable identifier for the schema (e.g., "Pet", "Error").
	Name string

	// IsCircular indicates that this schema contains circular references
	// and must be bounded during data generation.
	IsCircular bool
}
