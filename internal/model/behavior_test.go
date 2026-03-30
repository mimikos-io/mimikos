package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBehaviorType_String(t *testing.T) {
	tests := []struct {
		bt   BehaviorType
		want string
	}{
		{BehaviorCreate, "create"},
		{BehaviorFetch, "fetch"},
		{BehaviorList, "list"},
		{BehaviorUpdate, "update"},
		{BehaviorDelete, "delete"},
		{BehaviorGeneric, "generic"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.bt.String(), "BehaviorType.String() for %v", tt.bt)
	}
}

func TestBehaviorType_IsValid(t *testing.T) {
	assert.True(t, BehaviorCreate.IsValid())
	assert.True(t, BehaviorGeneric.IsValid())
	assert.False(t, BehaviorType("bogus").IsValid())
	assert.False(t, BehaviorType("").IsValid())
}

func TestParseBehaviorType(t *testing.T) {
	bt, err := ParseBehaviorType("create")
	require.NoError(t, err)
	assert.Equal(t, BehaviorCreate, bt)

	bt, err = ParseBehaviorType("generic")
	require.NoError(t, err)
	assert.Equal(t, BehaviorGeneric, bt)

	_, err = ParseBehaviorType("bogus")
	require.ErrorIs(t, err, ErrInvalidBehaviorType)

	_, err = ParseBehaviorType("")
	require.ErrorIs(t, err, ErrInvalidBehaviorType)
}

func TestScenario_String(t *testing.T) {
	tests := []struct {
		s    Scenario
		want string
	}{
		{ScenarioSuccess, "success"},
		{ScenarioValidationError, "validation_error"},
		{ScenarioNotFound, "not_found"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.s.String(), "Scenario.String() for %v", tt.s)
	}
}

func TestScenario_IsValid(t *testing.T) {
	assert.True(t, ScenarioSuccess.IsValid())
	assert.True(t, ScenarioNotFound.IsValid())
	assert.False(t, Scenario("bogus").IsValid())
}

func TestBehaviorEntry_Validate(t *testing.T) {
	t.Run("valid entry", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.NoError(t, entry.Validate())
	})

	t.Run("missing method", func(t *testing.T) {
		entry := BehaviorEntry{
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("missing path pattern", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid behavior type", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorType("bogus"),
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("empty scenarios", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid scenario in list", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess, Scenario("bogus")},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("zero success code", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "POST",
			PathPattern: "/pets",
			Type:        BehaviorCreate,
			Scenarios:   []Scenario{ScenarioSuccess},
			Source:      SourceHeuristic,
			Confidence:  0.8,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid source", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      Source("magic"),
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("confidence out of range", func(t *testing.T) {
		entry := BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        BehaviorList,
			Scenarios:   []Scenario{ScenarioSuccess},
			SuccessCode: 200,
			Source:      SourceHeuristic,
			Confidence:  1.5,
		}
		assert.Error(t, entry.Validate())
	})
}

func TestBehaviorMap_PutAndGet(t *testing.T) {
	bm := NewBehaviorMap()

	entry := BehaviorEntry{
		Method:      "GET",
		PathPattern: "/pets",
		Type:        BehaviorList,
		Scenarios:   []Scenario{ScenarioSuccess},
		SuccessCode: 200,
		Source:      SourceHeuristic,
		Confidence:  0.9,
	}
	bm.Put(entry)

	got, ok := bm.Get("GET", "/pets")
	require.True(t, ok)
	assert.Equal(t, BehaviorList, got.Type)

	_, ok = bm.Get("GET", "/nonexistent")
	assert.False(t, ok)

	_, ok = bm.Get("POST", "/pets")
	assert.False(t, ok)
}

func TestBehaviorMap_Len(t *testing.T) {
	bm := NewBehaviorMap()
	assert.Equal(t, 0, bm.Len())

	bm.Put(BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	assert.Equal(t, 1, bm.Len())

	bm.Put(BehaviorEntry{Method: "POST", PathPattern: "/pets"})
	assert.Equal(t, 2, bm.Len())

	// Same key overwrites, count stays the same.
	bm.Put(BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	assert.Equal(t, 2, bm.Len())
}

func TestBehaviorMap_Entries(t *testing.T) {
	bm := NewBehaviorMap()
	bm.Put(BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	bm.Put(BehaviorEntry{Method: "POST", PathPattern: "/pets"})
	bm.Put(BehaviorEntry{Method: "DELETE", PathPattern: "/pets/{petId}"})

	entries := bm.Entries()
	assert.Len(t, entries, 3)
}

func TestBehaviorMapKey(t *testing.T) {
	// Same method+path should produce the same key.
	k1 := BehaviorMapKey("GET", "/pets")
	k2 := BehaviorMapKey("GET", "/pets")
	assert.Equal(t, k1, k2)

	// Different method or path should differ.
	k3 := BehaviorMapKey("POST", "/pets")
	assert.NotEqual(t, k1, k3)

	k4 := BehaviorMapKey("GET", "/pets/{petId}")
	assert.NotEqual(t, k1, k4)
}

func TestCompiledSchema_Validate_NilSafe(t *testing.T) {
	// Nil receiver should not panic.
	var cs *CompiledSchema
	assert.NoError(t, cs.Validate("anything"))

	// Non-nil struct with nil Schema should not panic.
	cs = &CompiledSchema{Name: "test"}
	assert.NoError(t, cs.Validate("anything"))
}
