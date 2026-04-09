package generator

import (
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	// ErrUnsupportedType is returned when Generate is called with a schema whose
	// resolved type is not a primitive (object, array, or null).
	ErrUnsupportedType = errors.New("unsupported schema type for primitive generation")

	// recentDateStart and recentDateEnd define the range for generated date and
	// date-time values. A rolling 3-year window relative to the current system
	// year produces plausible-looking dates that stay current as time passes.
	// Computed once at package init — deterministic within a process lifetime.
	// Specs that need exact dates should use example values.
	//
	//nolint:gochecknoglobals // package-level values computed once at init from system clock
	recentDateStart = time.Date(time.Now().Year()-2, 1, 1, 0, 0, 0, 0, time.UTC)
	//nolint:gochecknoglobals
	recentDateEnd = time.Date(time.Now().Year(), 12, 31, 23, 59, 59, 0, time.UTC)
)

// JSON Schema type name constants.
const (
	typeString  = "string"
	typeInteger = "integer"
	typeNumber  = "number"
	typeBoolean = "boolean"
	typeNull    = "null"

	// defaultStringLen is the default length for generated strings when no
	// constraints are specified.
	defaultStringLen = 8

	// defaultIntMin and defaultIntMax bound integer generation when no
	// constraints are specified.
	defaultIntMin = 0
	defaultIntMax = 10000

	// defaultNumMin and defaultNumMax bound number generation when no
	// constraints are specified.
	defaultNumMin = 0.0
	defaultNumMax = 10000.0
	// exclusiveEpsilon is the small nudge applied to exclusive bounds to avoid
	// generating values exactly at the boundary.
	exclusiveEpsilon = 0.01
)

// PrimitiveGenerator produces deterministic values for JSON Schema primitive
// types (string, integer, number, boolean). Generic generation satisfies
// schema constraints (minLength, maximum, enum, format, etc.). Format-specific
// strings are truncated to maxLength when the constraint is present.
//
// When a semantic mapper is provided and the field name matches a known
// pattern, the semantic value is used if it satisfies the schema type.
// Semantic values bypass schema constraints — the field-name intent takes
// precedence over constraint satisfaction for mock data realism.
// The caller (typically the DataGenerator) may post-validate if needed.
type PrimitiveGenerator struct {
	semantic *SemanticMapper
}

// NewPrimitiveGenerator creates a generator with the given semantic mapper.
// If semantic is nil, all generation uses generic constraint-aware logic.
func NewPrimitiveGenerator(semantic *SemanticMapper) *PrimitiveGenerator {
	return &PrimitiveGenerator{semantic: semantic}
}

// Generate produces a deterministic value for a JSON Schema primitive type.
// The schema parameter provides type and constraint information. The seed
// controls deterministic output. The fieldName enables semantic matching.
//
// Returns the generated value and nil error on success. Returns
// [ErrUnsupportedType] if the schema type is not a supported primitive
// (object, array, null).
func (g *PrimitiveGenerator) Generate(schema *jsonschema.Schema, seed int64, fieldName string) (any, error) {
	// 1. Const short-circuit.
	if schema.Const != nil {
		return normalizeIntegerValue(*schema.Const, resolveType(schema)), nil
	}

	// 2. Enum short-circuit.
	if schema.Enum != nil && len(schema.Enum.Values) > 0 {
		return g.generateEnum(schema, seed)
	}

	typ := resolveType(schema)

	// 3. Example short-circuit — spec-provided example value.
	if len(schema.Examples) > 0 {
		example := schema.Examples[0]
		if typeCompatible(example, typ) {
			return normalizeIntegerValue(example, typ), nil
		}
		// Type mismatch: fall through to semantic/faker.
	}

	// 4. Semantic mapper — try before generic generation.
	if fieldName != "" && g.semantic != nil {
		if val, ok := g.trySemanticMatch(fieldName, typ, seed); ok {
			return val, nil
		}
	}

	// 5. Type-specific generation.
	switch typ {
	case typeString:
		return g.generateString(schema, seed), nil
	case typeInteger:
		return g.generateInteger(schema, seed), nil
	case typeNumber:
		return g.generateNumber(schema, seed), nil
	case typeBoolean:
		return g.generateBoolean(seed), nil
	default:
		return nil, ErrUnsupportedType
	}
}

// resolveType determines the JSON Schema type from the schema definition.
// Priority: explicit type → infer from constraints → default "string".
func resolveType(schema *jsonschema.Schema) string {
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

	// Infer from constraints.
	if schema.MinLength != nil || schema.MaxLength != nil || schema.Pattern != nil {
		return typeString
	}

	if schema.Minimum != nil || schema.Maximum != nil ||
		schema.ExclusiveMinimum != nil || schema.ExclusiveMaximum != nil ||
		schema.MultipleOf != nil {
		return typeNumber
	}

	// Default to string.
	return typeString
}

// generateEnum selects a deterministic value from the enum list.
// For integer-typed schemas, the selected value is normalized to int64
// regardless of the source Go type (int from YAML unmarshal, float64 from
// JSON unmarshal).
func (g *PrimitiveGenerator) generateEnum(schema *jsonschema.Schema, seed int64) (any, error) {
	values := schema.Enum.Values
	idx := absModLen(seed, len(values))
	val := values[idx]

	// Normalize numeric enum values to int64 for integer-typed schemas.
	typ := resolveType(schema)
	if typ == typeInteger {
		switch n := val.(type) {
		case int:
			return int64(n), nil
		case int64:
			return n, nil
		case float64:
			return int64(n), nil
		}
	}

	return val, nil
}

// trySemanticMatch attempts to use the semantic mapper for the given field name.
// Returns the value and true if a type-compatible semantic match was found.
func (g *PrimitiveGenerator) trySemanticMatch(fieldName, typ string, seed int64) (any, bool) {
	fn, ok := g.semantic.Match(fieldName)
	if !ok {
		return nil, false
	}

	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding
	val := fn(faker)

	if typeCompatible(val, typ) {
		return val, true
	}

	return nil, false
}

// typeCompatible checks whether a Go value is compatible with the target
// JSON Schema type.
func typeCompatible(val any, schemaType string) bool {
	switch schemaType {
	case typeString:
		_, ok := val.(string)

		return ok
	case typeInteger:
		switch v := val.(type) {
		case int:
			return true
		case int64:
			return true
		case float64:
			return v == math.Trunc(v)
		default:
			return false
		}
	case typeNumber:
		switch val.(type) {
		case int, int64, float64:
			return true
		default:
			return false
		}
	case typeBoolean:
		_, ok := val.(bool)

		return ok
	default:
		return false
	}
}

// generateString produces a deterministic string value respecting format and
// length constraints. Format-specific strings (email, uuid, etc.) are
// truncated to maxLength when the constraint is present.
func (g *PrimitiveGenerator) generateString(schema *jsonschema.Schema, seed int64) string {
	// Format-specific generation with length enforcement.
	if schema.Format != nil {
		if s, ok := generateStringFormat(schema.Format.Name, seed); ok {
			if schema.MaxLength != nil && len(s) > *schema.MaxLength {
				s = s[:*schema.MaxLength]
			}

			return s
		}
	}

	// Compute target length from constraints.
	length := defaultStringLen
	if schema.MinLength != nil && *schema.MinLength > length {
		length = *schema.MinLength
	}

	if schema.MaxLength != nil && *schema.MaxLength < length {
		length = *schema.MaxLength
	}

	// Short-circuit for maxLength: 0 — LetterN(0) returns 1 char (NB3).
	if length == 0 {
		return ""
	}

	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding

	return faker.LetterN(uint(length)) //nolint:gosec // length is always non-negative
}

// generateStringFormat produces a string value for a known JSON Schema format.
// Returns the value and true if the format is recognized, or ("", false) for
// unknown formats.
func generateStringFormat(format string, seed int64) (string, bool) {
	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding

	switch format {
	case "email":
		return faker.Email(), true
	case "uuid":
		return faker.UUID(), true
	case "date-time":
		return faker.DateRange(recentDateStart, recentDateEnd).Format("2006-01-02T15:04:05Z"), true
	case "date":
		return faker.DateRange(recentDateStart, recentDateEnd).Format("2006-01-02"), true
	case "uri", "uri-reference", "iri", "iri-reference":
		return faker.URL(), true
	case "hostname":
		return faker.DomainName(), true
	case "ipv4":
		return faker.IPv4Address(), true
	case "ipv6":
		return faker.IPv6Address(), true
	default:
		return "", false
	}
}

// generateInteger produces a deterministic integer value respecting min, max,
// exclusiveMin, exclusiveMax, and multipleOf constraints.
func (g *PrimitiveGenerator) generateInteger(schema *jsonschema.Schema, seed int64) int64 {
	minVal := int64(defaultIntMin)
	maxVal := int64(defaultIntMax)

	if schema.Minimum != nil {
		minVal = ratToInt64(schema.Minimum)
	}

	if schema.ExclusiveMinimum != nil {
		exMin := ratToInt64(schema.ExclusiveMinimum) + 1
		if exMin > minVal {
			minVal = exMin
		}
	}

	if schema.Maximum != nil {
		maxVal = ratToInt64(schema.Maximum)
	}

	if schema.ExclusiveMaximum != nil {
		exMax := ratToInt64(schema.ExclusiveMaximum) - 1
		if exMax < maxVal {
			maxVal = exMax
		}
	}

	if minVal > maxVal {
		maxVal = minVal
	}

	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding
	val := int64(faker.IntRange(int(minVal), int(maxVal)))

	// Snap to multipleOf if specified.
	if schema.MultipleOf != nil {
		mult := ratToInt64(schema.MultipleOf)
		if mult > 0 {
			val = snapToMultiple(val, mult, minVal, maxVal)
		}
	}

	return val
}

// snapToMultiple adjusts val to be a multiple of mult within [minVal, maxVal].
func snapToMultiple(val, mult, minVal, maxVal int64) int64 {
	// Round down to nearest multiple.
	result := (val / mult) * mult

	// Ensure >= minVal.
	if result < minVal {
		// Round up to the next multiple at or above minVal.
		result = ((minVal + mult - 1) / mult) * mult
	}

	// Ensure <= maxVal.
	if result > maxVal {
		result = (maxVal / mult) * mult
	}

	return result
}

// generateNumber produces a deterministic float64 value respecting numeric
// constraints.
func (g *PrimitiveGenerator) generateNumber(schema *jsonschema.Schema, seed int64) float64 {
	minVal := defaultNumMin
	maxVal := defaultNumMax

	if schema.Minimum != nil {
		minVal = ratToFloat64(schema.Minimum)
	}

	if schema.ExclusiveMinimum != nil {
		exMin := ratToFloat64(schema.ExclusiveMinimum) + exclusiveEpsilon
		if exMin > minVal {
			minVal = exMin
		}
	}

	if schema.Maximum != nil {
		maxVal = ratToFloat64(schema.Maximum)
	}

	if schema.ExclusiveMaximum != nil {
		exMax := ratToFloat64(schema.ExclusiveMaximum) - exclusiveEpsilon
		if exMax < maxVal {
			maxVal = exMax
		}
	}

	if minVal > maxVal {
		maxVal = minVal
	}

	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding
	val := faker.Float64Range(minVal, maxVal)

	// Snap to multipleOf if specified.
	if schema.MultipleOf != nil {
		mult := ratToFloat64(schema.MultipleOf)
		if mult > 0 {
			val = snapToMultipleFloat(val, mult, minVal, maxVal)
		}
	}

	return val
}

// snapToMultipleFloat adjusts val to be a multiple of mult within
// [minVal, maxVal].
func snapToMultipleFloat(val, mult, minVal, maxVal float64) float64 {
	result := math.Round(val/mult) * mult

	if result < minVal {
		result = math.Ceil(minVal/mult) * mult
	}

	if result > maxVal {
		result = math.Floor(maxVal/mult) * mult
	}

	return result
}

// --- Boolean generation ---

func (g *PrimitiveGenerator) generateBoolean(seed int64) bool {
	faker := gofakeit.New(uint64(seed)) //nolint:gosec // intentional bit-cast for seeding

	return faker.Bool()
}

// --- Helpers ---

// ratToFloat64 converts a *big.Rat to float64.
func ratToFloat64(r *big.Rat) float64 {
	f, _ := r.Float64()

	return f
}

// ratToInt64 converts a *big.Rat to int64 via integer division.
func ratToInt64(r *big.Rat) int64 {
	// Use Num/Denom for integer division — truncates toward zero.
	return r.Num().Int64() / r.Denom().Int64()
}

// normalizeIntegerValue converts int and float64 values to int64 when the
// target schema type is "integer". This ensures consistent return types
// across all integer generation paths (const, enum, example, faker).
// YAML unmarshal produces Go int for integers; JSON unmarshal produces
// float64. Both are normalized to int64 to match generateInteger's contract.
// Non-integer types are returned unchanged.
func normalizeIntegerValue(val any, typ string) any {
	if typ != typeInteger {
		return val
	}

	switch n := val.(type) {
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return val
	}
}

// absModLen returns a non-negative index derived from seed modulo length.
// Uses unsigned conversion to avoid two's complement overflow on math.MinInt64
// (where -MinInt64 == MinInt64, producing a negative result with signed negation).
func absModLen(seed int64, length int) int {
	return int(uint64(seed) % uint64(length)) //nolint:gosec // result always fits in int since length is int
}
