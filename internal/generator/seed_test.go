package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldSeed_Determinism(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	seed1 := FieldSeed(reqSeed, "name")
	seed2 := FieldSeed(reqSeed, "name")

	assert.Equal(t, seed1, seed2, "same request seed + field path must produce same seed")
}

func TestFieldSeed_FieldIsolation(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	// Record seeds for two fields.
	nameSeed := FieldSeed(reqSeed, "name")
	emailSeed := FieldSeed(reqSeed, "email")

	// Adding a third field must not change the existing two.
	nameSeedAfter := FieldSeed(reqSeed, "name")
	emailSeedAfter := FieldSeed(reqSeed, "email")
	_ = FieldSeed(reqSeed, "newField")

	assert.Equal(t, nameSeed, nameSeedAfter, "name seed must not change when other fields are added")
	assert.Equal(t, emailSeed, emailSeedAfter, "email seed must not change when other fields are added")
}

func TestFieldSeed_PathDifferentiation(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	citySeed := FieldSeed(reqSeed, "address/city")
	stateSeed := FieldSeed(reqSeed, "address/state")

	assert.NotEqual(t, citySeed, stateSeed, "different field paths must produce different seeds")
}

func TestFieldSeed_NestedPathAmbiguity(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	seed1 := FieldSeed(reqSeed, "a/bc")
	seed2 := FieldSeed(reqSeed, "ab/c")

	assert.NotEqual(t, seed1, seed2, "a/bc and ab/c must produce different seeds")
}

func TestFieldSeed_ArrayIndices(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	seed0 := FieldSeed(reqSeed, "tags/0")
	seed1 := FieldSeed(reqSeed, "tags/1")

	assert.NotEqual(t, seed0, seed1, "different array indices must produce different seeds")
}

func TestFieldSeed_RootField(t *testing.T) {
	reqSeed := Fingerprint("GET", "/pets", nil, nil)

	// Empty field path represents a top-level scalar schema.
	seed := FieldSeed(reqSeed, "")
	assert.NotZero(t, seed, "empty field path must produce a valid non-zero seed")
}

func TestFieldSeed_DifferentRequestSeeds(t *testing.T) {
	reqSeed1 := Fingerprint("GET", "/pets", nil, nil)
	reqSeed2 := Fingerprint("GET", "/users", nil, nil)

	seed1 := FieldSeed(reqSeed1, "name")
	seed2 := FieldSeed(reqSeed2, "name")

	assert.NotEqual(t, seed1, seed2, "same field path with different request seeds must differ")
}

func TestFieldSeed_OrderIndependence(t *testing.T) {
	reqSeed := Fingerprint("POST", "/users", nil, nil)

	// Compute seeds in one order.
	fields := []string{"email", "name", "age", "address/city"}

	seedsForward := make(map[string]int64, len(fields))
	for _, f := range fields {
		seedsForward[f] = FieldSeed(reqSeed, f)
	}

	// Compute in reverse order — must produce identical seeds.
	seedsReverse := make(map[string]int64, len(fields))
	for i := len(fields) - 1; i >= 0; i-- {
		seedsReverse[fields[i]] = FieldSeed(reqSeed, fields[i])
	}

	assert.Equal(t, seedsForward, seedsReverse, "field seed computation must not depend on call order")
}
