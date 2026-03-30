package generator

import (
	"math"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Primitive type tests (8.4.1) ---

func TestGenerateStringBasic(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("string")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok, "expected string, got %T", val)
	assert.NotEmpty(t, s)
}

func TestGenerateStringFormats(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)

	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, val string)
	}{
		{
			name:   "email",
			format: "email",
			check: func(t *testing.T, val string) {
				t.Helper()
				assert.Contains(t, val, "@", "email must contain @")
				assert.Contains(t, val, ".", "email must contain .")
			},
		},
		{
			name:   "uuid",
			format: "uuid",
			check: func(t *testing.T, val string) {
				t.Helper()
				uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
				assert.Regexp(t, uuidRe, val)
			},
		},
		{
			name:   "date-time",
			format: "date-time",
			check: func(t *testing.T, val string) {
				t.Helper()
				_, err := time.Parse(time.RFC3339, val)
				assert.NoError(t, err, "date-time must parse as RFC 3339")
			},
		},
		{
			name:   "date",
			format: "date",
			check: func(t *testing.T, val string) {
				t.Helper()
				_, err := time.Parse("2006-01-02", val)
				assert.NoError(t, err, "date must parse as YYYY-MM-DD")
			},
		},
		{
			name:   "uri",
			format: "uri",
			check: func(t *testing.T, val string) {
				t.Helper()
				assert.True(t, strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://"),
					"uri must start with http(s)://")
			},
		},
		{
			name:   "hostname",
			format: "hostname",
			check: func(t *testing.T, val string) {
				t.Helper()
				assert.Contains(t, val, ".", "hostname must contain a dot")
			},
		},
		{
			name:   "ipv4",
			format: "ipv4",
			check: func(t *testing.T, val string) {
				t.Helper()
				ip := net.ParseIP(val)
				assert.NotNil(t, ip, "must be valid IP")
				assert.NotNil(t, ip.To4(), "must be IPv4")
			},
		},
		{
			name:   "ipv6",
			format: "ipv6",
			check: func(t *testing.T, val string) {
				t.Helper()
				ip := net.ParseIP(val)
				assert.NotNil(t, ip, "must be valid IP")
				assert.Contains(t, val, ":", "ipv6 must contain colons")
			},
		},
		{
			name:   "unknown-format-falls-through",
			format: "x-custom-format",
			check: func(t *testing.T, val string) {
				t.Helper()
				assert.NotEmpty(t, val, "unknown format should produce generic string")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := &jsonschema.Schema{
				Types:  newTypes("string"),
				Format: newFormat(tt.format),
			}

			val, err := g.Generate(schema, 12345, "")
			require.NoError(t, err)

			s, ok := val.(string)
			require.True(t, ok, "expected string, got %T", val)
			tt.check(t, s)
		})
	}
}

func TestGenerateIntegerBasic(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("integer")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(int64)
	assert.True(t, ok, "expected int64, got %T", val)
}

func TestGenerateNumberBasic(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("number")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(float64)
	assert.True(t, ok, "expected float64, got %T", val)
}

func TestGenerateBooleanBasic(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("boolean")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(bool)
	assert.True(t, ok, "expected bool, got %T", val)
}

func TestGenerateDeterminism(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)

	tests := []struct {
		name   string
		schema *jsonschema.Schema
	}{
		{"string", &jsonschema.Schema{Types: newTypes("string")}},
		{"integer", &jsonschema.Schema{Types: newTypes("integer")}},
		{"number", &jsonschema.Schema{Types: newTypes("number")}},
		{"boolean", &jsonschema.Schema{Types: newTypes("boolean")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const seed int64 = 99999

			v1, err1 := g.Generate(tt.schema, seed, "")
			require.NoError(t, err1)

			v2, err2 := g.Generate(tt.schema, seed, "")
			require.NoError(t, err2)

			assert.Equal(t, v1, v2, "same seed must produce same value")
		})
	}
}

func TestGenerateDifferentiation(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("string")}

	v1, err1 := g.Generate(schema, 1, "")
	require.NoError(t, err1)

	v2, err2 := g.Generate(schema, 2, "")
	require.NoError(t, err2)

	assert.NotEqual(t, v1, v2, "different seeds should produce different values")
}

// --- Constraint tests (8.4.2) ---

func TestGenerateStringMinLength(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("string"),
		MinLength: ptr(10),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(s), 10)
}

func TestGenerateStringMaxLength(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("string"),
		MaxLength: ptr(5),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.LessOrEqual(t, len(s), 5)
}

func TestGenerateStringExactLength(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("string"),
		MinLength: ptr(3),
		MaxLength: ptr(3),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.Len(t, s, 3)
}

func TestGenerateIntegerMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:   newTypes("integer"),
		Minimum: newRat(100),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, n, int64(100))
}

func TestGenerateIntegerMaximum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:   newTypes("integer"),
		Maximum: newRat(10),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.LessOrEqual(t, n, int64(10))
}

func TestGenerateIntegerExclusiveMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:            newTypes("integer"),
		ExclusiveMinimum: newRat(0),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.Positive(t, n)
}

func TestGenerateIntegerExclusiveMaximum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:            newTypes("integer"),
		ExclusiveMaximum: newRat(100),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.Less(t, n, int64(100))
}

func TestGenerateIntegerMinMax(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:   newTypes("integer"),
		Minimum: newRat(5),
		Maximum: newRat(10),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, n, int64(5))
	assert.LessOrEqual(t, n, int64(10))
}

func TestGenerateIntegerMultipleOf(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:      newTypes("integer"),
		MultipleOf: newRat(3),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.Equal(t, int64(0), n%3, "value %d must be a multiple of 3", n)
}

func TestGenerateIntegerMultipleOfWithMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:      newTypes("integer"),
		MultipleOf: newRat(5),
		Minimum:    newRat(12),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(int64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, n, int64(12))
	assert.Equal(t, int64(0), n%5, "value %d must be a multiple of 5", n)
}

func TestGenerateNumberMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:   newTypes("number"),
		Minimum: newRatF(1.5),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, n, 1.5)
}

func TestGenerateNumberMaximum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:   newTypes("number"),
		Maximum: newRatF(99.9),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(float64)
	require.True(t, ok)
	assert.LessOrEqual(t, n, 99.9)
}

func TestGenerateNumberExclusiveMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:            newTypes("number"),
		ExclusiveMinimum: newRatF(0.0),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(float64)
	require.True(t, ok)
	assert.Greater(t, n, 0.0)
}

func TestGenerateNumberMultipleOf(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:      newTypes("number"),
		MultipleOf: newRatF(0.25),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	n, ok := val.(float64)
	require.True(t, ok)

	// Check that n / 0.25 is a whole number (within float epsilon).
	quotient := n / 0.25
	assert.InDelta(t, float64(int64(quotient)), quotient, 0.001,
		"value %f must be a multiple of 0.25", n)
}

func TestGenerateEnumString(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types: newTypes("string"),
		Enum:  newEnum("active", "inactive", "pending"),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.Contains(t, []string{"active", "inactive", "pending"}, s)
}

func TestGenerateEnumInteger(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)

	t.Run("float64-values-from-JSON", func(t *testing.T) {
		t.Parallel()

		// JSON unmarshal produces float64 for all numbers.
		schema := &jsonschema.Schema{
			Types: newTypes("integer"),
			Enum:  newEnum(float64(1), float64(2), float64(3)),
		}

		val, err := g.Generate(schema, 42, "")
		require.NoError(t, err)

		n, ok := val.(int64)
		require.True(t, ok, "expected int64, got %T (%v)", val, val)
		assert.Contains(t, []int64{1, 2, 3}, n)
	})

	t.Run("int-values-from-YAML", func(t *testing.T) {
		t.Parallel()

		// YAML unmarshal (go.yaml.in/yaml/v4) produces Go int for integers.
		// This is what santhosh-tekuri/jsonschema stores when compiling from YAML.
		schema := &jsonschema.Schema{
			Types: newTypes("integer"),
			Enum:  newEnum(1, 2, 3),
		}

		val, err := g.Generate(schema, 42, "")
		require.NoError(t, err)

		n, ok := val.(int64)
		require.True(t, ok, "expected int64, got %T (%v)", val, val)
		assert.Contains(t, []int64{1, 2, 3}, n)
	})
}

func TestGenerateConst(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	constVal := any("fixed-value")
	schema := &jsonschema.Schema{
		Const: &constVal,
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)
	assert.Equal(t, "fixed-value", val)
}

// --- Type inference tests ---

func TestGenerateInferStringFromMinLength(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		MinLength: ptr(5),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok, "expected string from minLength hint, got %T", val)
	assert.GreaterOrEqual(t, len(s), 5)
}

func TestGenerateInferNumberFromMinimum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Minimum: newRat(0),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(float64)
	assert.True(t, ok, "expected float64 from minimum hint, got %T", val)
}

func TestGenerateInferFromEnum(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Enum: newEnum("x", "y"),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)
	assert.Contains(t, []string{"x", "y"}, val)
}

func TestGenerateNoTypeNoHints(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(string)
	assert.True(t, ok, "expected string as default type, got %T", val)
}

// --- Semantic integration tests (8.4.5) ---

func TestGenerateSemanticEmail(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewPrimitiveGenerator(sm)
	schema := &jsonschema.Schema{Types: newTypes("string")}

	val, err := g.Generate(schema, 42, "email")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.Contains(t, s, "@", "semantic email field should produce email-like value")
}

func TestGenerateSemanticPrice(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewPrimitiveGenerator(sm)
	schema := &jsonschema.Schema{Types: newTypes("number")}

	val, err := g.Generate(schema, 42, "price")
	require.NoError(t, err)

	n, ok := val.(float64)
	require.True(t, ok)
	assert.Greater(t, n, 0.0, "price should be positive")
}

func TestGenerateSemanticTypeMismatchSkipped(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewPrimitiveGenerator(sm)
	// Schema is boolean, but field name "email" maps to string generator.
	// Semantic match should be skipped due to type incompatibility.
	schema := &jsonschema.Schema{Types: newTypes("boolean")}

	val, err := g.Generate(schema, 42, "email")
	require.NoError(t, err)

	_, ok := val.(bool)
	assert.True(t, ok, "type mismatch should fall through to boolean generation, got %T", val)
}

func TestGenerateNoFieldName(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewPrimitiveGenerator(sm)
	schema := &jsonschema.Schema{Types: newTypes("string")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	// With no field name, should produce generic string (no @ from email).
	// This is a probabilistic check — generic strings almost never contain @.
	assert.NotEmpty(t, s)
}

func TestGenerateNilMapper(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("string")}

	val, err := g.Generate(schema, 42, "email")
	require.NoError(t, err)

	_, ok := val.(string)
	assert.True(t, ok, "nil mapper should produce generic string, got %T", val)
}

// --- Error case tests ---

func TestGenerateUnsupportedTypes(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)

	unsupported := []struct {
		name   string
		schema *jsonschema.Schema
	}{
		{"object", &jsonschema.Schema{Types: newTypes("object")}},
		{"array", &jsonschema.Schema{Types: newTypes("array")}},
		{"null", &jsonschema.Schema{Types: newTypes("null")}},
	}

	for _, tt := range unsupported {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := g.Generate(tt.schema, 42, "")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrUnsupportedType)
		})
	}
}

// --- Enum determinism across seeds ---

func TestGenerateEnumDeterministic(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types: newTypes("string"),
		Enum:  newEnum("a", "b", "c"),
	}

	const seed int64 = 777

	v1, err := g.Generate(schema, seed, "")
	require.NoError(t, err)

	v2, err := g.Generate(schema, seed, "")
	require.NoError(t, err)

	assert.Equal(t, v1, v2, "enum selection must be deterministic for same seed")
}

func TestGenerateEnumVariation(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types: newTypes("string"),
		Enum:  newEnum("a", "b", "c", "d", "e"),
	}

	// With 5 enum values and many seeds, we should see at least 2 distinct values.
	seen := make(map[any]bool)

	for seed := int64(0); seed < 20; seed++ {
		val, err := g.Generate(schema, seed, "")
		require.NoError(t, err)

		seen[val] = true
	}

	assert.GreaterOrEqual(t, len(seen), 2, "enum should produce variation across seeds")
}

// --- Review regression tests ---

// B1: absModLen must not panic on math.MinInt64 (two's complement overflow).
func TestGenerateEnumMinInt64Seed(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types: newTypes("string"),
		Enum:  newEnum("a", "b", "c"),
	}

	// math.MinInt64 caused negation overflow in the original absModLen:
	// -MinInt64 == MinInt64 in two's complement, producing a negative index.
	val, err := g.Generate(schema, math.MinInt64, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.Contains(t, []string{"a", "b", "c"}, s)
}

// B3: Format strings must respect maxLength when the constraint is present.
func TestGenerateFormatRespectsMaxLength(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)

	tests := []struct {
		name      string
		format    string
		maxLength int
	}{
		{"email-truncated", "email", 15},
		{"uuid-truncated", "uuid", 20},
		{"uri-truncated", "uri", 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := &jsonschema.Schema{
				Types:     newTypes("string"),
				Format:    newFormat(tt.format),
				MaxLength: ptr(tt.maxLength),
			}

			val, err := g.Generate(schema, 42, "")
			require.NoError(t, err)

			s, ok := val.(string)
			require.True(t, ok)
			assert.LessOrEqual(t, len(s), tt.maxLength,
				"format %q must respect maxLength %d, got len=%d", tt.format, tt.maxLength, len(s))
		})
	}
}

// NB3: maxLength: 0 must produce empty string (LetterN(0) returns 1 char).
func TestGenerateStringMaxLengthZero(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("string"),
		MaxLength: ptr(0),
	}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	s, ok := val.(string)
	require.True(t, ok)
	assert.Empty(t, s, "maxLength: 0 must produce empty string")
}

// NB6: Nullable type ["string", "null"] must generate string, not error.
func TestGenerateNullableString(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("string", "null")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(string)
	assert.True(t, ok, "nullable string should generate string, got %T", val)
}

// NB6: Nullable type ["integer", "null"] must generate integer, not error.
func TestGenerateNullableInteger(t *testing.T) {
	t.Parallel()

	g := NewPrimitiveGenerator(nil)
	schema := &jsonschema.Schema{Types: newTypes("integer", "null")}

	val, err := g.Generate(schema, 42, "")
	require.NoError(t, err)

	_, ok := val.(int64)
	assert.True(t, ok, "nullable integer should generate int64, got %T", val)
}
