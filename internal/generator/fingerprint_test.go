package generator_test

import (
	"net/url"
	"testing"

	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/stretchr/testify/assert"
)

func TestFingerprint_Determinism(t *testing.T) {
	query := url.Values{"limit": {"10"}, "offset": {"0"}}
	body := []byte(`{"name":"Fido"}`)

	seed1 := generator.Fingerprint("GET", "/pets", query, body)
	seed2 := generator.Fingerprint("GET", "/pets", query, body)

	assert.Equal(t, seed1, seed2, "identical inputs must produce identical seeds")
}

func TestFingerprint_QueryOrderIndependence(t *testing.T) {
	q1 := url.Values{"a": {"1"}, "b": {"2"}}
	q2 := url.Values{"b": {"2"}, "a": {"1"}}

	seed1 := generator.Fingerprint("GET", "/pets", q1, nil)
	seed2 := generator.Fingerprint("GET", "/pets", q2, nil)

	assert.Equal(t, seed1, seed2, "query parameter key order must not affect fingerprint")
}

func TestFingerprint_QueryMultiValueOrderIndependence(t *testing.T) {
	q1 := url.Values{"a": {"2", "1"}}
	q2 := url.Values{"a": {"1", "2"}}

	seed1 := generator.Fingerprint("GET", "/pets", q1, nil)
	seed2 := generator.Fingerprint("GET", "/pets", q2, nil)

	assert.Equal(t, seed1, seed2, "multi-value order must not affect fingerprint")
}

func TestFingerprint_JSONBodyKeyOrderIndependence(t *testing.T) {
	body1 := []byte(`{"a":1,"b":2}`)
	body2 := []byte(`{"b":2,"a":1}`)

	seed1 := generator.Fingerprint("POST", "/pets", nil, body1)
	seed2 := generator.Fingerprint("POST", "/pets", nil, body2)

	assert.Equal(t, seed1, seed2, "JSON key order must not affect fingerprint")
}

func TestFingerprint_NestedJSONKeyOrderIndependence(t *testing.T) {
	body1 := []byte(`{"outer":{"x":1,"y":2},"name":"test"}`)
	body2 := []byte(`{"name":"test","outer":{"y":2,"x":1}}`)

	seed1 := generator.Fingerprint("POST", "/pets", nil, body1)
	seed2 := generator.Fingerprint("POST", "/pets", nil, body2)

	assert.Equal(t, seed1, seed2, "nested JSON key order must not affect fingerprint")
}

func TestFingerprint_MethodDifferentiation(t *testing.T) {
	seed1 := generator.Fingerprint("GET", "/pets", nil, nil)
	seed2 := generator.Fingerprint("POST", "/pets", nil, nil)

	assert.NotEqual(t, seed1, seed2, "different methods must produce different seeds")
}

func TestFingerprint_PathDifferentiation(t *testing.T) {
	seed1 := generator.Fingerprint("GET", "/pets", nil, nil)
	seed2 := generator.Fingerprint("GET", "/users", nil, nil)

	assert.NotEqual(t, seed1, seed2, "different paths must produce different seeds")
}

func TestFingerprint_QueryDifferentiation(t *testing.T) {
	q1 := url.Values{"limit": {"10"}}
	q2 := url.Values{"limit": {"20"}}

	seed1 := generator.Fingerprint("GET", "/pets", q1, nil)
	seed2 := generator.Fingerprint("GET", "/pets", q2, nil)

	assert.NotEqual(t, seed1, seed2, "different query values must produce different seeds")
}

func TestFingerprint_BodyDifferentiation(t *testing.T) {
	body1 := []byte(`{"name":"Fido"}`)
	body2 := []byte(`{"name":"Rex"}`)

	seed1 := generator.Fingerprint("POST", "/pets", nil, body1)
	seed2 := generator.Fingerprint("POST", "/pets", nil, body2)

	assert.NotEqual(t, seed1, seed2, "different bodies must produce different seeds")
}

func TestFingerprint_NilQuery(t *testing.T) {
	// Must not panic and must produce a valid seed.
	seed := generator.Fingerprint("GET", "/pets", nil, nil)
	assert.NotZero(t, seed, "nil query must produce a valid non-zero seed")
}

func TestFingerprint_NilBody(t *testing.T) {
	seed := generator.Fingerprint("GET", "/pets", url.Values{"a": {"1"}}, nil)
	assert.NotZero(t, seed, "nil body must produce a valid non-zero seed")
}

func TestFingerprint_NonJSONBody(t *testing.T) {
	body := []byte("this is not json")

	seed1 := generator.Fingerprint("POST", "/upload", nil, body)
	seed2 := generator.Fingerprint("POST", "/upload", nil, body)

	assert.Equal(t, seed1, seed2, "non-JSON body must be deterministic")
}

func TestFingerprint_EmptyBodyEqualsNilBody(t *testing.T) {
	seed1 := generator.Fingerprint("GET", "/pets", nil, nil)
	seed2 := generator.Fingerprint("GET", "/pets", nil, []byte{})

	assert.Equal(t, seed1, seed2, "nil and empty body must produce same fingerprint")
}

func TestFingerprint_EmptyQueryEqualsNilQuery(t *testing.T) {
	seed1 := generator.Fingerprint("GET", "/pets", nil, nil)
	seed2 := generator.Fingerprint("GET", "/pets", url.Values{}, nil)

	assert.Equal(t, seed1, seed2, "nil and empty query must produce same fingerprint")
}

func TestFingerprint_QueryPresenceChangesFingerprint(t *testing.T) {
	seed1 := generator.Fingerprint("GET", "/pets", nil, nil)
	seed2 := generator.Fingerprint("GET", "/pets", url.Values{"limit": {"10"}}, nil)

	assert.NotEqual(t, seed1, seed2, "adding query params must change fingerprint")
}

func TestFingerprint_BodyPresenceChangesFingerprint(t *testing.T) {
	seed1 := generator.Fingerprint("POST", "/pets", nil, nil)
	seed2 := generator.Fingerprint("POST", "/pets", nil, []byte(`{"name":"Fido"}`))

	assert.NotEqual(t, seed1, seed2, "adding body must change fingerprint")
}

func TestFingerprint_SeparatorCollision(t *testing.T) {
	// Path containing the separator character must not collide with
	// component boundaries. Regression test for B1 review finding.
	seed1 := generator.Fingerprint("GET", "/a||c", nil, nil)
	seed2 := generator.Fingerprint("GET", "/a", nil, []byte("c"))

	assert.NotEqual(t, seed1, seed2, "pipe in path must not collide with component separator")
}

func TestFingerprint_LargeIntegerPrecision(t *testing.T) {
	// Integers beyond float64 precision (>2^53) must not collide due to
	// rounding. Regression test for B2 review finding.
	body1 := []byte(`{"id":9999999999999999}`)
	body2 := []byte(`{"id":10000000000000000}`)

	seed1 := generator.Fingerprint("POST", "/tx", nil, body1)
	seed2 := generator.Fingerprint("POST", "/tx", nil, body2)

	assert.NotEqual(t, seed1, seed2, "large integers must not lose precision in canonicalization")
}

func TestFingerprint_PathParameterDifferentiation(t *testing.T) {
	// Different path parameter values must produce different seeds.
	// This is the primary runtime use case for fingerprinting.
	seed1 := generator.Fingerprint("GET", "/users/123", nil, nil)
	seed2 := generator.Fingerprint("GET", "/users/456", nil, nil)

	assert.NotEqual(t, seed1, seed2, "different path parameters must produce different seeds")
}
