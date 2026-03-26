package parser

import "github.com/pb33f/libopenapi/datamodel/high/base"

// IsNullable returns true if the schema allows null values.
// Handles both OpenAPI 3.0 (nullable: true) and 3.1 (type includes "null").
func IsNullable(schema *base.Schema) bool {
	if schema == nil {
		return false
	}

	// 3.0 style: nullable: true
	if schema.Nullable != nil && *schema.Nullable {
		return true
	}

	// 3.1 style: type array includes "null"
	for _, t := range schema.Type {
		if t == "null" {
			return true
		}
	}

	return false
}

// PrimaryType returns the non-null type string for the schema.
// For 3.0: returns schema.Type[0].
// For 3.1: returns the first type that isn't "null".
// Returns "" if no type is defined (implies object when Properties is non-nil).
func PrimaryType(schema *base.Schema) string {
	if schema == nil {
		return ""
	}

	for _, t := range schema.Type {
		if t != "null" {
			return t
		}
	}

	return ""
}
