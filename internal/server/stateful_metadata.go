package server

import (
	"log/slog"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
)

// annotateStatefulMetadata enriches behavior entries with wrapper keys,
// list array keys, and ID field hints needed for stateful mode.
// Only runs when mode is stateful. Modifies entries in-place via bm.Put().
//
// The three annotation concerns are independent of each other:
//   - WrapperKey/ListArrayKey depend on response schemas (per-entry)
//   - IDFieldHint depends on path patterns and behavior types (cross-entry)
//
// None reads another's output, so they can run in a single pass.
func annotateStatefulMetadata(bm *model.BehaviorMap, logger *slog.Logger) {
	// Build resource type → item path param index for IDFieldHint computation.
	// This pre-scan is needed because create entries must reference their
	// fetch variant's path param, which may appear later in the entries slice.
	paramByResource := buildParamIndex(bm.Entries())

	for _, e := range bm.Entries() {
		changed := false

		// delete and generic don't need metadata
		//nolint:exhaustive
		switch e.Type {
		case model.BehaviorCreate, model.BehaviorFetch, model.BehaviorUpdate:
			e.WrapperKey = detectWrapperKey(e.ResponseSchemas, e.SuccessCode)
			changed = true
		case model.BehaviorList:
			e.ListArrayKey = detectListArrayKey(e.ResponseSchemas, e.SuccessCode, logger)
			changed = true
		}

		if hint := computeIDFieldHint(e, paramByResource); hint != "" {
			e.IDFieldHint = hint
			changed = true
		}

		if changed {
			bm.Put(e)
		}
	}
}

// resolveRef follows $ref chains to the target schema. The jsonschema v6
// compiler represents $ref as schema.Ref (pointer to resolved target).
// The referencing node's own Types/Properties may be nil — type information
// lives on the target.
func resolveRef(s *jsonschema.Schema) *jsonschema.Schema {
	for s != nil && s.Ref != nil {
		s = s.Ref
	}

	return s
}

// hasType checks whether the schema's Types includes the given type string.
// Also checks allOf branches, since jsonschema v6 preserves allOf composition
// rather than merging — a schema with allOf:[{type: object, ...}] has nil Types
// at the top level.
//
// oneOf/anyOf are intentionally NOT traversed. Research finding (§6.4):
// "oneOf/anyOf at response top level: not observed in any studied spec for
// single-resource responses." The correct default is to treat such schemas
// as flat (no wrapper).
//
// Nil-safe: returns false if schema or Types is nil.
func hasType(s *jsonschema.Schema, t string) bool {
	if s == nil {
		return false
	}

	if s.Types != nil {
		for _, st := range s.Types.ToStrings() {
			if st == t {
				return true
			}
		}
	}

	// Check allOf branches — jsonschema v6 may store type info in allOf
	// instead of merging into the top-level Types field.
	for _, branch := range s.AllOf {
		resolved := resolveRef(branch)
		if resolved != nil && resolved.Types != nil {
			for _, st := range resolved.Types.ToStrings() {
				if st == t {
					return true
				}
			}
		}
	}

	return false
}

// resolveResponseSchema retrieves the compiled response schema for the given
// success code, falling back to the default response (key 0).
func resolveResponseSchema(schemas map[int]*model.CompiledSchema, successCode int) *model.CompiledSchema {
	if s := schemas[successCode]; s != nil {
		return s
	}

	return schemas[0]
}

// detectWrapperKey inspects the success response schema for a single-property
// object wrapper pattern (e.g., Asana's {data: {resource}}).
// Returns the wrapper key name, or "" for flat responses.
func detectWrapperKey(schemas map[int]*model.CompiledSchema, successCode int) string {
	cs := resolveResponseSchema(schemas, successCode)
	if cs == nil || cs.Schema == nil {
		return ""
	}

	s := resolveRef(cs.Schema)

	// Must be an object type with exactly one defined property.
	if !hasType(s, "object") || len(s.Properties) != 1 {
		return ""
	}

	// Reject open-ended objects (additionalProperties: true).
	// A single property + arbitrary extra keys is not a wrapper pattern.
	if b, ok := s.AdditionalProperties.(bool); ok && b {
		return ""
	}

	// The single property must resolve to an object type.
	for key, propSchema := range s.Properties {
		resolved := resolveRef(propSchema)
		if hasType(resolved, "object") {
			return key
		}
	}

	return ""
}

// detectListArrayKey inspects the list response schema to find the array-typed
// property that holds the collection. Returns the property name, or "" for
// bare array (top-level type: array) schemas.
func detectListArrayKey(schemas map[int]*model.CompiledSchema, successCode int, logger *slog.Logger) string {
	cs := resolveResponseSchema(schemas, successCode)
	if cs == nil || cs.Schema == nil {
		return ""
	}

	s := resolveRef(cs.Schema)

	// Top-level array → bare array, no wrapper needed.
	if hasType(s, "array") {
		return ""
	}

	// Must be object type.
	if !hasType(s, "object") {
		return ""
	}

	// Find properties that resolve to array type.
	var arrayKey string

	arrayCount := 0

	for key, propSchema := range s.Properties {
		resolved := resolveRef(propSchema)
		if hasType(resolved, "array") {
			arrayKey = key
			arrayCount++
		}
	}

	// Exactly one array property → use it.
	if arrayCount == 1 {
		return arrayKey
	}

	// Multiple array properties → ambiguous, fall back to bare array.
	if arrayCount > 1 && logger != nil {
		logger.Warn("list schema has multiple array properties, falling back to bare array",
			"count", arrayCount,
		)
	}

	return ""
}

// buildParamIndex creates a lookup from resource type to the last path
// parameter name, derived from fetch/update/delete entries.
func buildParamIndex(entries []model.BehaviorEntry) map[string]string {
	paramByResource := make(map[string]string)

	for _, e := range entries {
		if e.Type == model.BehaviorFetch || e.Type == model.BehaviorUpdate || e.Type == model.BehaviorDelete {
			rt := state.ResourceType(e.PathPattern)
			param := state.LastPathParam(e.PathPattern)

			if param != "" {
				paramByResource[rt] = param
			}
		}
	}

	return paramByResource
}

// computeIDFieldHint derives the body field name expected to hold the
// resource ID. Only produces a hint when suffix-stripping yields a result
// (e.g., "project_gid" → "gid"). When stripping fails, returns "" —
// the raw param name is NOT used as a fallback because it could match
// a non-ID body field (e.g., param "order" matching body["order"] which
// is a nested object, not an ID).
func computeIDFieldHint(e model.BehaviorEntry, paramByResource map[string]string) string {
	rt := state.ResourceType(e.PathPattern)

	paramName, ok := paramByResource[rt]
	if !ok {
		return ""
	}

	// Use the leaf collection name for StripResourcePrefix — it needs a
	// singular word like "sections", not a namespace path like "projects/*/sections".
	leaf := state.LeafCollection(e.PathPattern)

	remainder := state.StripResourcePrefix(paramName, leaf)
	if remainder != "" {
		return remainder
	}

	// No prefix to strip. Don't fall back to raw param name — it's either:
	// - Already a direct body field match (handled by Strategy 1: exact match)
	// - A resource-name param like "customer" (handled by Strategy 4: id fallback)
	// Setting it as a hint would waste Strategy 3 or match a non-ID field.
	return ""
}
