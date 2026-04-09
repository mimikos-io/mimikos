// Package generator implements deterministic, schema-aware data generation
// for Mimikos mock responses. It provides request fingerprinting, per-field
// sub-seeding, semantic field-name mapping, and schema-complete data generation
// as the foundation for producing realistic, reproducible API response data.
//
// The generation pipeline:
//
//	Request → Fingerprint → DataGenerator.Generate(schema, seed) → response payload
//	  └── per field: FieldSeed → SemanticMapper / PrimitiveGenerator → value
//
// All outputs are deterministic: identical requests always produce identical
// response data across process restarts.
package generator

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strconv"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// defaultMaxDepth is the maximum recursion depth for nested object/array
	// generation. Three levels covers the vast majority of real API schemas.
	defaultMaxDepth = 3

	// defaultArrayLen is the number of items generated for arrays when no
	// minItems/maxItems constraints are specified.
	defaultArrayLen = 3

	// mapEntryCount is the number of entries generated for map-type schemas
	// (object with additionalProperties but no defined properties).
	mapEntryCount = 3

	// mapKeyLen is the length of generated map keys for map-type schemas.
	mapKeyLen = 6

	// maxUniqueRetries caps the number of retry attempts per item when
	// uniqueItems is required and a duplicate is generated.
	maxUniqueRetries = 10

	// maxRecursion is a hard safety limit on total call depth (structural +
	// composition) to prevent stack overflow on circular allOf/oneOf/anyOf
	// chains where no intermediate object/array increments structural depth.
	maxRecursion = 64

	// Warning messages for depth and recursion termination. Extracted to constants
	// to stay within the line-length limit at the call site.
	warnCircularSchema = "circular schema detected — field generated as null to prevent " +
		"infinite recursion; check spec for circular $ref chains"
	warnDepthObject = "schema depth limit reached — object field generated as null; " +
		"increase --max-depth to generate nested objects"
	warnDepthArray = "schema depth limit reached — array field generated as empty; " +
		"increase --max-depth to generate nested arrays"
)

type (
	// DataGenerator produces deterministic, schema-complete response data by
	// walking a JSON Schema tree. It delegates primitive leaf generation to
	// [PrimitiveGenerator] and handles object properties, array items,
	// polymorphism (allOf/oneOf/anyOf), and circular reference termination.
	//
	// Two depth counters prevent infinite recursion:
	//
	//   - depth (structural): incremented only by object properties and array
	//     items. Composition keywords (allOf/oneOf/anyOf) are depth-neutral —
	//     they describe what an object is made of, not deeper nesting. This
	//     allows specs with deep allOf inheritance chains (e.g., TaskResponse →
	//     TaskBase → TaskCompact → AsanaResource) to fully resolve without
	//     hitting the depth limit. Objects at the depth limit produce nil
	//     (JSON null). Arrays at the depth limit produce empty slices (JSON []).
	//
	//   - recurse (absolute): incremented on every call into generate,
	//     regardless of kind. Acts as a hard safety net (maxRecursion = 64)
	//     against stack overflow from circular composition chains
	//     (e.g., A.allOf → B, B.allOf → A) that never pass through an
	//     object or array to increment structural depth.
	//
	// All outputs are deterministic: identical schemas with identical seeds
	// always produce identical data structures.
	DataGenerator struct {
		primitive *PrimitiveGenerator
		maxDepth  int
		logger    *slog.Logger
	}

	// discardHandler is a slog.Handler that discards all log output.
	discardHandler struct{}
)

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// NewDataGenerator creates a generator with the given semantic mapper and
// maximum recursion depth. If maxDepth is 0, the default (3) is used.
// If logger is nil, a no-op logger is used.
func NewDataGenerator(semantic *SemanticMapper, maxDepth int, logger *slog.Logger) *DataGenerator {
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}

	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	return &DataGenerator{
		primitive: NewPrimitiveGenerator(semantic),
		maxDepth:  maxDepth,
		logger:    logger,
	}
}

// Generate produces a complete data structure for the given schema.
// The seed controls deterministic output. Returns the generated value
// (map[string]any for objects, []any for arrays, or a primitive) and
// nil error on success.
func (g *DataGenerator) Generate(schema *jsonschema.Schema, seed int64) (any, error) {
	return g.generate(schema, seed, "", 0, 0)
}

// generate is the recursive core. It resolves $ref, polymorphism, and type
// dispatch, threading depth for structural termination and recurse for
// absolute stack-overflow protection.
//
//nolint:cyclop
func (g *DataGenerator) generate(
	schema *jsonschema.Schema, seed int64, fieldName string, depth int, recurse int,
) (any, error) {
	if schema == nil {
		return nil, nil //nolint:nilnil // nil schema produces nil value
	}

	// Hard recursion guard: prevents stack overflow on circular compositions
	// (e.g., A.allOf→B, B.allOf→A) where no object/array increments depth.
	// Logged at Warn because hitting this limit signals a likely spec defect
	// (purely circular composition chain), not normal depth management.
	if recurse >= maxRecursion {
		g.logger.Warn(warnCircularSchema, "field", fieldName, "recurse", recurse, "schema", schemaID(schema))

		return nil, nil //nolint:nilnil // recursion limit termination
	}

	// 1. Follow $ref — alias, does not increment depth.
	if schema.Ref != nil {
		return g.generate(schema.Ref, seed, fieldName, depth, recurse+1)
	}

	// 2. Const / enum short-circuit (delegate to primitive).
	if schema.Const != nil || (schema.Enum != nil && len(schema.Enum.Values) > 0) {
		return g.primitive.Generate(schema, seed, fieldName)
	}

	// 3. Polymorphism — checked before type dispatch.
	// Composition keywords (allOf/oneOf/anyOf) are depth-neutral: they
	// describe "what this object is made of," not structural nesting.
	// Only object properties and array items increment depth. This
	// prevents artificial depth exhaustion on specs with deep allOf
	// inheritance chains.
	if len(schema.AllOf) > 0 {
		return g.generateAllOf(schema, seed, depth, recurse+1)
	}

	if len(schema.OneOf) > 0 {
		return g.generateBranch(schema.OneOf, seed, fieldName, depth, recurse+1)
	}

	if len(schema.AnyOf) > 0 {
		return g.generateBranch(schema.AnyOf, seed, fieldName, depth, recurse+1)
	}

	// 4. Type dispatch.
	typ := resolveComplexType(schema)

	switch typ {
	case typeObject:
		if depth >= g.maxDepth {
			g.logger.Warn(warnDepthObject,
				"field", fieldName, "depth", depth, "maxDepth", g.maxDepth,
				"schema", schemaID(schema))

			return nil, nil //nolint:nilnil // depth termination — nil is the documented sentinel
		}

		return g.generateObject(schema, seed, depth, recurse)
	case typeArray:
		if depth >= g.maxDepth {
			g.logger.Warn(warnDepthArray,
				"field", fieldName, "depth", depth, "maxDepth", g.maxDepth,
				"schema", schemaID(schema))

			return []any{}, nil // depth termination — empty array
		}

		return g.generateArray(schema, seed, depth, recurse)
	case typeNull:
		return nil, nil //nolint:nilnil // null type produces nil
	default:
		// Primitive types: string, integer, number, boolean.
		return g.primitive.Generate(schema, seed, fieldName)
	}
}

// --- Object generation ---

// generateObject produces a map[string]any with all defined properties.
// Properties are iterated in sorted order for deterministic output.
func (g *DataGenerator) generateObject(
	schema *jsonschema.Schema, seed int64, depth int, recurse int,
) (map[string]any, error) {
	// Map-type schema: no defined properties, but additionalProperties is a schema.
	if len(schema.Properties) == 0 {
		if ap, ok := schema.AdditionalProperties.(*jsonschema.Schema); ok {
			return g.generateMapEntries(ap, seed, depth, recurse)
		}

		return map[string]any{}, nil
	}

	// Collect and sort property names for deterministic iteration.
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}

	sort.Strings(names)

	result := make(map[string]any, len(names))

	for _, name := range names {
		propSchema := schema.Properties[name]
		fieldSeed := FieldSeed(seed, name)

		val, err := g.generate(propSchema, fieldSeed, name, depth+1, recurse+1)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}

		result[name] = val
	}

	return result, nil
}

// generateMapEntries produces a small number of entries for map-type schemas
// (object with additionalProperties but no defined properties).
func (g *DataGenerator) generateMapEntries(
	valueSchema *jsonschema.Schema, seed int64, depth int, recurse int,
) (map[string]any, error) {
	result := make(map[string]any, mapEntryCount)

	for i := range mapEntryCount {
		keyPath := fmt.Sprintf("_key_%d", i)
		keySeed := FieldSeed(seed, keyPath)
		keyFaker := gofakeit.New(uint64(keySeed)) //nolint:gosec // intentional bit-cast for seeding
		key := keyFaker.LetterN(mapKeyLen)

		valPath := fmt.Sprintf("_val_%d", i)
		valSeed := FieldSeed(seed, valPath)

		val, err := g.generate(valueSchema, valSeed, "", depth+1, recurse+1)
		if err != nil {
			return nil, fmt.Errorf("map entry %d: %w", i, err)
		}

		result[key] = val
	}

	return result, nil
}

// --- Array generation ---

// generateArray produces a []any with items generated from the item schema.
// Tuple schemas (prefixItems or draft-04 Items as []*Schema) generate
// per-position items from the corresponding schema.
func (g *DataGenerator) generateArray(schema *jsonschema.Schema, seed int64, depth int, recurse int) ([]any, error) {
	// Tuple schemas: per-position item schemas.
	if tupleSchemas := resolveTupleSchemas(schema); len(tupleSchemas) > 0 {
		return g.generateTuple(tupleSchemas, seed, depth, recurse)
	}

	// Homogeneous arrays: single item schema for all items.
	itemSchema := resolveItemSchema(schema)
	if itemSchema == nil {
		return []any{}, nil
	}

	length := arrayLength(schema)

	items := make([]any, 0, length)

	for i := range length {
		itemSeed := FieldSeed(seed, strconv.Itoa(i))

		item, err := g.generate(itemSchema, itemSeed, "", depth+1, recurse+1)
		if err != nil {
			return nil, fmt.Errorf("array item %d: %w", i, err)
		}

		if schema.UniqueItems && containsValue(items, item) {
			item, err = g.retryUniqueItem(itemSchema, seed, i, items, depth, recurse)
			if err != nil {
				return nil, fmt.Errorf("array item %d unique retry: %w", i, err)
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// generateTuple produces a []any with per-position items from the given
// schemas (one schema per array position).
func (g *DataGenerator) generateTuple(
	schemas []*jsonschema.Schema, seed int64, depth int, recurse int,
) ([]any, error) {
	items := make([]any, 0, len(schemas))

	for i, itemSchema := range schemas {
		itemSeed := FieldSeed(seed, strconv.Itoa(i))

		val, err := g.generate(itemSchema, itemSeed, "", depth+1, recurse+1)
		if err != nil {
			return nil, fmt.Errorf("tuple item %d: %w", i, err)
		}

		items = append(items, val)
	}

	return items, nil
}

// retryUniqueItem generates alternative values for a duplicate array item.
// Returns the first unique value found, or the last attempt if all retries
// produce duplicates (best-effort). Propagates generation errors.
func (g *DataGenerator) retryUniqueItem(
	itemSchema *jsonschema.Schema, seed int64, index int, existing []any, depth int, recurse int,
) (any, error) {
	var lastCandidate any

	for attempt := 1; attempt <= maxUniqueRetries; attempt++ {
		retrySeed := FieldSeed(seed, fmt.Sprintf("%d_retry_%d", index, attempt))

		candidate, err := g.generate(itemSchema, retrySeed, "", depth+1, recurse+1)
		if err != nil {
			return nil, fmt.Errorf("retry %d: %w", attempt, err)
		}

		if !containsValue(existing, candidate) {
			return candidate, nil
		}

		lastCandidate = candidate
	}

	// Best-effort: return last attempt even if duplicate.
	return lastCandidate, nil
}

// containsValue checks if items contains val using deep equality.
func containsValue(items []any, val any) bool {
	for _, existing := range items {
		if reflect.DeepEqual(existing, val) {
			return true
		}
	}

	return false
}

// resolveTupleSchemas returns per-position schemas for tuple-style arrays
// (prefixItems or draft-04 Items as []*Schema). Returns nil for homogeneous
// arrays.
func resolveTupleSchemas(schema *jsonschema.Schema) []*jsonschema.Schema {
	// Draft 2020-12 tuple.
	if len(schema.PrefixItems) > 0 {
		return schema.PrefixItems
	}

	// Draft-04 style tuple: Items is []*Schema.
	if schema.Items != nil {
		if schemas, ok := schema.Items.([]*jsonschema.Schema); ok && len(schemas) > 0 {
			return schemas
		}
	}

	return nil
}

// resolveItemSchema extracts the single item schema for homogeneous arrays.
// Returns nil if no item schema is defined. Tuple schemas are handled
// separately by resolveTupleSchemas.
func resolveItemSchema(schema *jsonschema.Schema) *jsonschema.Schema {
	// Draft 2020-12.
	if schema.Items2020 != nil {
		return schema.Items2020
	}

	// Draft-07 style: Items as *Schema (single schema for all items).
	if schema.Items != nil {
		if s, ok := schema.Items.(*jsonschema.Schema); ok {
			return s
		}
	}

	return nil
}

// arrayLength computes the target array length from minItems/maxItems.
func arrayLength(schema *jsonschema.Schema) int {
	length := defaultArrayLen

	if schema.MinItems != nil && *schema.MinItems > length {
		length = *schema.MinItems
	}

	if schema.MaxItems != nil && *schema.MaxItems < length {
		length = *schema.MaxItems
	}

	return length
}

// --- Polymorphism ---

// generateAllOf merges data from all sub-schemas. If the parent schema has
// top-level properties, they are generated first as the base.
func (g *DataGenerator) generateAllOf(schema *jsonschema.Schema, seed int64, depth int, recurse int) (any, error) {
	if len(schema.AllOf) == 0 {
		return nil, nil //nolint:nilnil // empty allOf
	}

	result := make(map[string]any)

	// Generate top-level properties first (if any).
	if len(schema.Properties) > 0 {
		base, err := g.generateObject(schema, seed, depth, recurse)
		if err != nil {
			return nil, err
		}

		mergeMaps(result, base)
	}

	// Merge each allOf sub-schema. Each sub-schema gets a unique seed
	// derived from its index so sibling oneOf blocks within separate allOf
	// entries can select different branches independently.
	for i, sub := range schema.AllOf {
		subSeed := FieldSeed(seed, fmt.Sprintf("_allOf_%d", i))

		val, err := g.generate(sub, subSeed, "", depth, recurse+1)
		if err != nil {
			return nil, err
		}

		if m, ok := val.(map[string]any); ok {
			mergeMaps(result, m)
		} else if val != nil {
			// Non-object allOf (rare: e.g., allOf with a single primitive constraint).
			return val, nil
		}
	}

	return result, nil
}

// generateBranch selects one branch deterministically from oneOf or anyOf.
// Null-typed branches are filtered out when non-null alternatives exist,
// consistent with resolveComplexType which already skips "null" in inline
// type arrays (e.g., Types: ["object", "null"] → generates object).
// A mock server showing data is always more useful than showing null.
func (g *DataGenerator) generateBranch(
	schemas []*jsonschema.Schema, seed int64, fieldName string, depth int, recurse int,
) (any, error) {
	if len(schemas) == 0 {
		return nil, nil //nolint:nilnil // empty oneOf/anyOf
	}

	// Prefer non-null branches. Only activates when at least one null
	// branch is present and at least one non-null branch exists.
	if filtered := nonNullBranches(schemas); len(filtered) > 0 {
		schemas = filtered
	}

	idx := absModLen(seed, len(schemas))

	return g.generate(schemas[idx], seed, fieldName, depth, recurse+1)
}

// isNullBranch returns true if the schema resolves to exclusively the null
// type. Follows $ref transparently. Returns false for nil schemas and schemas
// without an explicit type (which default to string/object via inference).
func isNullBranch(schema *jsonschema.Schema) bool {
	if schema == nil {
		return false
	}

	if schema.Ref != nil {
		return isNullBranch(schema.Ref)
	}

	if schema.Types == nil {
		return false
	}

	types := schema.Types.ToStrings()

	return len(types) == 1 && types[0] == typeNull
}

// nonNullBranches returns the subset of schemas that are not null-typed.
// Returns nil if all branches are null-typed, allowing the caller to fall
// back to the original list.
func nonNullBranches(schemas []*jsonschema.Schema) []*jsonschema.Schema {
	var result []*jsonschema.Schema

	for _, s := range schemas {
		if !isNullBranch(s) {
			result = append(result, s)
		}
	}

	return result
}

// mergeMaps copies all entries from src into dst (last-write-wins).
func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// --- Type resolution ---

// JSON Schema type name constants for complex types.
const (
	typeObject = "object"
	typeArray  = "array"
)

// resolveComplexType determines the JSON Schema type, including object and
// array. Falls back to resolveType() for primitive type inference.
func resolveComplexType(schema *jsonschema.Schema) string {
	// Explicit type declared.
	if schema.Types != nil {
		for _, t := range schema.Types.ToStrings() {
			if t != typeNull {
				return t
			}
		}
		// Only "null" type.
		return typeNull
	}

	// Infer from structural signals.
	if len(schema.Properties) > 0 {
		return typeObject
	}

	if schema.AdditionalProperties != nil {
		if _, ok := schema.AdditionalProperties.(*jsonschema.Schema); ok {
			return typeObject
		}
	}

	if schema.Items2020 != nil || schema.Items != nil || len(schema.PrefixItems) > 0 {
		return typeArray
	}

	// Fall back to primitive type inference.
	return resolveType(schema)
}

// schemaID returns a short identifier for a schema, useful for log messages.
// Prefers the schema Location (JSON pointer), falling back to "anonymous".
func schemaID(schema *jsonschema.Schema) string {
	if schema.Location != "" {
		return schema.Location
	}

	return "anonymous"
}
