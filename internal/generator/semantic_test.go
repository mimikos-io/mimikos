package generator

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemanticMapper_KnownNamesMatch(t *testing.T) {
	m := NewSemanticMapper()

	knownNames := []string{
		"email", "firstName", "city", "url", "createdAt",
		"price", "isActive", "ip", "username", "country",
	}

	for _, name := range knownNames {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			assert.True(t, ok, "expected match for %q", name)
			assert.NotNil(t, fn, "generator function must not be nil for %q", name)
		})
	}
}

func TestSemanticMapper_CaseInsensitivity(t *testing.T) {
	m := NewSemanticMapper()

	variants := []string{"email", "Email", "EMAIL", "eMaIl"}

	// All variants must match.
	for _, v := range variants {
		_, ok := m.Match(v)
		assert.True(t, ok, "expected match for %q", v)
	}

	// All variants with the same seed must produce the same value.
	fn1, _ := m.Match("email")
	fn2, _ := m.Match("EMAIL")

	f := gofakeit.New(99)
	val1 := fn1(f)
	f = gofakeit.New(99)
	val2 := fn2(f)

	assert.Equal(t, val1, val2, "case variants with same seed must produce same value")
}

func TestSemanticMapper_SnakeCaseNormalization(t *testing.T) {
	m := NewSemanticMapper()

	tests := []struct {
		snakeCase string
		camelCase string
	}{
		{"first_name", "firstName"},
		{"created_at", "createdAt"},
		{"is_active", "isActive"},
		{"email_address", "emailAddress"},
		{"ip_address", "ipAddress"},
	}

	for _, tt := range tests {
		t.Run(tt.snakeCase, func(t *testing.T) {
			_, snakeOK := m.Match(tt.snakeCase)
			_, camelOK := m.Match(tt.camelCase)
			assert.Equal(t, snakeOK, camelOK,
				"%q and %q must both match or both not match", tt.snakeCase, tt.camelCase)
		})
	}
}

func TestSemanticMapper_KebabCaseNormalization(t *testing.T) {
	m := NewSemanticMapper()

	_, ok := m.Match("first-name")
	assert.True(t, ok, "kebab-case first-name must match")
}

func TestSemanticMapper_UnknownNameFallthrough(t *testing.T) {
	m := NewSemanticMapper()

	unknowns := []string{"fooBarBaz", "xyzzy", "qwerty123", "somethingRandom"}

	for _, name := range unknowns {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			assert.False(t, ok, "expected no match for %q", name)
			assert.Nil(t, fn, "generator function must be nil for unknown %q", name)
		})
	}
}

func TestSemanticMapper_Determinism(t *testing.T) {
	m := NewSemanticMapper()
	fn, ok := m.Match("email")
	require.True(t, ok)

	// Same seed must produce same value.
	f1 := gofakeit.New(12345)
	val1 := fn(f1)

	f2 := gofakeit.New(12345)
	val2 := fn(f2)

	assert.Equal(t, val1, val2, "same seed must produce identical value")
}

func TestSemanticMapper_Differentiation(t *testing.T) {
	m := NewSemanticMapper()
	fn, ok := m.Match("email")
	require.True(t, ok)

	f1 := gofakeit.New(111)
	val1 := fn(f1)

	f2 := gofakeit.New(222)
	val2 := fn(f2)

	assert.NotEqual(t, val1, val2, "different seeds should produce different values")
}

func TestSemanticMapper_CategoryCoverage(t *testing.T) {
	m := NewSemanticMapper()

	// uuidPattern matches v4 UUIDs and most hex-formatted UUIDs from gofakeit.
	uuidPattern := regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
	)

	tests := []struct {
		fieldName string
		validate  func(t *testing.T, val any)
	}{
		{"uuid", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "uuid must be a string, got %T", val)
			assert.Regexp(t, uuidPattern, s)
		}},
		{"firstName", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "firstName must be a string, got %T", val)
			assert.NotEmpty(t, s)
			assert.True(t, isAlphaString(s), "firstName must be alphabetic, got %q", s)
		}},
		{"email", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "email must be a string, got %T", val)

			_, err := mail.ParseAddress(s)
			require.NoError(t, err, "email must be parseable: %q", s)
		}},
		{"city", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "city must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"createdAt", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "createdAt must be a string, got %T", val)

			_, err := time.Parse(time.RFC3339, s)
			require.NoError(t, err, "createdAt must be RFC3339: %q", s)
		}},
		{"price", func(t *testing.T, val any) {
			t.Helper()

			f, ok := val.(float64)
			require.True(t, ok, "price must be float64, got %T", val)
			assert.GreaterOrEqual(t, f, 0.0, "price must be non-negative")
		}},
		{"currency", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "currency must be a string, got %T", val)
			assert.Len(t, s, 3, "currency code must be 3 chars: %q", s)
		}},
		{"url", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "url must be a string, got %T", val)

			u, err := url.Parse(s)
			require.NoError(t, err, "url must be parseable: %q", s)
			assert.True(t, u.Scheme == "http" || u.Scheme == "https", "url must have http(s) scheme: %q", s)
		}},
		{"description", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "description must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"isActive", func(t *testing.T, val any) {
			t.Helper()

			_, ok := val.(bool)
			assert.True(t, ok, "isActive must be bool, got %T", val)
		}},
		{"ip", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "ip must be a string, got %T", val)
			assert.NotNil(t, net.ParseIP(s), "ip must be a valid IP address: %q", s)
		}},
		{"count", func(t *testing.T, val any) {
			t.Helper()

			n, ok := val.(int)
			require.True(t, ok, "count must be int, got %T", val)
			assert.Positive(t, n, "count must be positive")
		}},
		{"color", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "color must be a string, got %T", val)
			assert.Regexp(t, `^#[0-9a-fA-F]{6}$`, s, "color must be hex: %q", s)
		}},
		{"company", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "company must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"jobTitle", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "jobTitle must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"phone", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "phone must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"latitude", func(t *testing.T, val any) {
			t.Helper()

			f, ok := val.(float64)
			require.True(t, ok, "latitude must be float64, got %T", val)
			assert.InDelta(t, 0, f, 90, "latitude must be in [-90, 90]")
		}},
		{"language", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "language must be a string, got %T", val)
			assert.NotEmpty(t, s)
		}},
		{"filename", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "filename must be a string, got %T", val)
			assert.Contains(t, s, ".", "filename must have an extension")
		}},
		{"slug", func(t *testing.T, val any) {
			t.Helper()

			s, ok := val.(string)
			require.True(t, ok, "slug must be a string, got %T", val)
			assert.Regexp(t, `^[a-z0-9]+(-[a-z0-9]+)*$`, s, "slug must be lowercase-hyphenated: %q", s)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			fn, ok := m.Match(tt.fieldName)
			require.True(t, ok, "expected match for %q", tt.fieldName)

			f := gofakeit.New(42)
			val := fn(f)
			tt.validate(t, val)
		})
	}
}

func TestSemanticMapper_MinimumEntryCount(t *testing.T) {
	m := NewSemanticMapper()

	assert.GreaterOrEqual(t, m.Len(), 100,
		"semantic mapper must have at least 100 field-name entries")
}

// isAlphaString returns true if the string contains only Unicode letters and spaces.
func isAlphaString(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && r != ' ' && r != '-' && r != '\'' {
			return false
		}
	}

	return true
}

// TestSemanticMapper_TimeFields verifies date-only fields produce date format (not datetime).
func TestSemanticMapper_TimeFields(t *testing.T) {
	m := NewSemanticMapper()
	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	dateOnlyFields := []string{"date", "birthday", "dob", "dueDate"}
	for _, name := range dateOnlyFields {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			require.True(t, ok, "expected match for %q", name)

			f := gofakeit.New(42)
			val := fn(f)

			s, ok := val.(string)
			require.True(t, ok, "%q must produce string, got %T", name, val)
			assert.Regexp(t, datePattern, s, "%q must produce date format YYYY-MM-DD: %q", name, s)
		})
	}
}

// TestSemanticMapper_IDFields verifies various ID field patterns match.
func TestSemanticMapper_IDFields(t *testing.T) {
	m := NewSemanticMapper()

	idFields := []string{"id", "uuid", "userId", "orderId", "accountId", "sessionId", "requestId"}
	for _, name := range idFields {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			require.True(t, ok, "expected match for %q", name)

			f := gofakeit.New(42)
			val := fn(f)

			s, ok := val.(string)
			require.True(t, ok, "%q must produce string, got %T", name, val)
			assert.NotEmpty(t, s)

			// Must look like a UUID.
			assert.Contains(t, s, "-", "%q should produce UUID-like value: %q", name, s)
		})
	}
}

// TestSemanticMapper_BooleanFields verifies boolean flag fields return bool type.
func TestSemanticMapper_BooleanFields(t *testing.T) {
	m := NewSemanticMapper()

	boolFields := []string{
		"isActive", "is_deleted", "enabled", "verified", "published", "archived",
	}
	for _, name := range boolFields {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			require.True(t, ok, "expected match for %q", name)

			f := gofakeit.New(42)
			val := fn(f)
			_, ok = val.(bool)
			assert.True(t, ok, "%q must produce bool, got %T (%v)", name, val, val)
		})
	}
}

// TestSemanticMapper_FinancialFieldsPrecision verifies financial fields use 2 decimal places.
func TestSemanticMapper_FinancialFieldsPrecision(t *testing.T) {
	m := NewSemanticMapper()

	financialFields := []string{"price", "amount", "total", "balance", "cost"}
	for _, name := range financialFields {
		t.Run(name, func(t *testing.T) {
			fn, ok := m.Match(name)
			require.True(t, ok, "expected match for %q", name)

			f := gofakeit.New(42)
			val := fn(f)

			fv, ok := val.(float64)
			require.True(t, ok, "%q must produce float64, got %T", name, val)

			// Check that it has at most 2 decimal places.
			s := fmt.Sprintf("%.10f", fv)
			parts := strings.Split(s, ".")
			require.Len(t, parts, 2)

			// Trim trailing zeros from decimal part.
			decimals := strings.TrimRight(parts[1], "0")
			assert.LessOrEqual(t, len(decimals), 2,
				"%q must have at most 2 decimal places: %v", name, fv)
		})
	}
}
