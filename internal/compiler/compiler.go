// Package compiler converts OpenAPI schemas into compiled JSON Schema
// validators using santhosh-tekuri/jsonschema. It works from the raw spec
// document to preserve $ref relationships and avoid circular reference
// issues that arise from serializing resolved schemas.
package compiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v4" // v4 over v3: unmarshals to map[string]any (v3 uses map[any]any for some cases)
)

// baseURL is the synthetic URL used to register the spec document
// with the jsonschema compiler. Schemas are compiled relative to this URL.
const baseURL = "spec.json"

// Sentinel errors for compiler failures.
var (
	// ErrEmptyInput is returned when the spec bytes are empty or nil.
	ErrEmptyInput = errors.New("compiler: empty input")

	// ErrInvalidSpec is returned when the spec bytes cannot be parsed as YAML.
	ErrInvalidSpec = errors.New("compiler: invalid spec")

	// ErrCompile is returned when the jsonschema compiler fails to compile a schema.
	ErrCompile = errors.New("compiler: schema compilation failed")
)

// SchemaCompiler compiles OpenAPI schemas into JSON Schema validators.
// Create one per spec using New(), then call Compile() for each schema pointer.
type SchemaCompiler struct {
	compiler *jsonschema.Compiler
}

// New creates a SchemaCompiler from raw OpenAPI spec bytes.
// The version string determines whether OpenAPI 3.0 normalization is applied
// (nullable, boolean exclusiveMinimum/Maximum).
func New(specBytes []byte, version string) (*SchemaCompiler, error) {
	if len(specBytes) == 0 {
		return nil, ErrEmptyInput
	}

	// Parse YAML into generic map.
	var doc any
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}

	// Normalize example → examples for all OpenAPI versions so the
	// jsonschema compiler preserves spec-provided example values.
	normalizeExamples(doc)

	// Normalize OpenAPI 3.0 schemas to JSON Schema 2020-12.
	if strings.HasPrefix(version, "3.0") {
		normalizeOpenAPI30(doc)
	}

	// Create jsonschema compiler and register the spec document.
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()

	if err := c.AddResource(baseURL, doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}

	return &SchemaCompiler{compiler: c}, nil
}

// Compile compiles a schema at the given JSON pointer within the spec.
// The pointer should be a fragment like "#/components/schemas/Pet" or
// "#/paths/~1pets/get/responses/200/content/application~1json/schema".
// The name and isCircular fields are passed through to the returned CompiledSchema.
func (sc *SchemaCompiler) Compile(pointer, name string, isCircular bool) (*model.CompiledSchema, error) {
	// Build the full URL: "spec.json#/components/schemas/Pet"
	url := baseURL + pointer

	compiled, err := sc.compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrCompile, pointer, err)
	}

	return &model.CompiledSchema{
		Name:       name,
		IsCircular: isCircular,
		Schema:     compiled,
	}, nil
}

// normalizeExamples converts OpenAPI "example" (singular) to JSON Schema
// "examples" (array) so the jsonschema compiler preserves example values.
// This runs for all OpenAPI versions — even 3.1 spec authors commonly use
// the "example" keyword since it is the established OpenAPI convention.
//
// A looksLikeSchema guard prevents the function from modifying container
// maps (like "properties" or "components/schemas") whose keys could
// coincidentally be "example".
func normalizeExamples(v any) {
	obj, ok := v.(map[string]any)
	if !ok {
		// Recurse into arrays.
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				normalizeExamples(item)
			}
		}

		return
	}

	// Only normalize maps that look like JSON Schema objects.
	if looksLikeSchema(obj) {
		if ex, hasExample := obj["example"]; hasExample {
			if _, hasExamples := obj["examples"]; !hasExamples {
				obj["examples"] = []any{ex}
			}

			delete(obj, "example")
		}
	}

	// Recurse into all child values.
	for _, child := range obj {
		normalizeExamples(child)
	}
}

// looksLikeSchema returns true if the map contains at least one key that
// is a recognized JSON Schema keyword — indicating it is a schema object
// rather than a container map (properties, components/schemas, paths, etc.).
func looksLikeSchema(obj map[string]any) bool {
	for _, key := range []string{
		"type", "$ref", "allOf", "oneOf", "anyOf",
		"enum", "const", "format", "properties", "items",
		"minimum", "maximum", "minLength", "maxLength", "pattern",
	} {
		if _, ok := obj[key]; ok {
			return true
		}
	}

	return false
}

// normalizeOpenAPI30 recursively walks a generic YAML/JSON document and
// converts OpenAPI 3.0 schema keywords to JSON Schema 2020-12 equivalents.
//
// Normalizations applied:
//   - nullable: true → type becomes ["<type>", "null"]
//   - exclusiveMinimum: true (boolean) → exclusiveMinimum: <minimum value>
//   - exclusiveMaximum: true (boolean) → exclusiveMaximum: <maximum value>
func normalizeOpenAPI30(v any) {
	switch val := v.(type) {
	case map[string]any:
		normalizeSchemaObject(val)

		// Recurse into all values.
		for _, child := range val {
			normalizeOpenAPI30(child)
		}
	case []any:
		for _, item := range val {
			normalizeOpenAPI30(item)
		}
	}
}

// normalizeSchemaObject applies OpenAPI 3.0 → JSON Schema 2020-12
// normalization to a single map that may be a schema object.
func normalizeSchemaObject(obj map[string]any) {
	// nullable: true → type: ["<type>", "null"]
	if nullable, ok := obj["nullable"].(bool); ok && nullable {
		if t, ok := obj["type"].(string); ok {
			// Simple case: type field present → add "null" to type array.
			obj["type"] = []any{t, "null"}
		} else if ref, hasRef := obj["$ref"]; hasRef {
			// nullable + $ref (no type): common in 3.0 specs (e.g., Stripe expandable fields).
			// Convert to anyOf: [{$ref: ...}, {type: "null"}].
			delete(obj, "$ref")
			obj["anyOf"] = []any{
				map[string]any{"$ref": ref},
				map[string]any{"type": "null"},
			}
		}

		delete(obj, "nullable")
	}

	// exclusiveMinimum: true (boolean) → exclusiveMinimum: <minimum value>
	if exclusive, ok := obj["exclusiveMinimum"].(bool); ok {
		if exclusive {
			if minVal, hasMin := obj["minimum"]; hasMin {
				obj["exclusiveMinimum"] = minVal
				delete(obj, "minimum")
			}
		} else {
			// exclusiveMinimum: false — just remove it, keep minimum as-is.
			delete(obj, "exclusiveMinimum")
		}
	}

	// exclusiveMaximum: true (boolean) → exclusiveMaximum: <maximum value>
	if exclusive, ok := obj["exclusiveMaximum"].(bool); ok {
		if exclusive {
			if maxVal, hasMax := obj["maximum"]; hasMax {
				obj["exclusiveMaximum"] = maxVal
				delete(obj, "maximum")
			}
		} else {
			delete(obj, "exclusiveMaximum")
		}
	}
}
