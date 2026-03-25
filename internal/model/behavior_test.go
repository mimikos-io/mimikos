package model_test

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBehaviorType_String(t *testing.T) {
	tests := []struct {
		bt   model.BehaviorType
		want string
	}{
		{model.BehaviorCreate, "create"},
		{model.BehaviorFetch, "fetch"},
		{model.BehaviorList, "list"},
		{model.BehaviorUpdate, "update"},
		{model.BehaviorDelete, "delete"},
		{model.BehaviorGeneric, "generic"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.bt.String(), "BehaviorType.String() for %v", tt.bt)
	}
}

func TestBehaviorType_IsValid(t *testing.T) {
	assert.True(t, model.BehaviorCreate.IsValid())
	assert.True(t, model.BehaviorGeneric.IsValid())
	assert.False(t, model.BehaviorType("bogus").IsValid())
	assert.False(t, model.BehaviorType("").IsValid())
}

func TestParseBehaviorType(t *testing.T) {
	bt, err := model.ParseBehaviorType("create")
	require.NoError(t, err)
	assert.Equal(t, model.BehaviorCreate, bt)

	bt, err = model.ParseBehaviorType("generic")
	require.NoError(t, err)
	assert.Equal(t, model.BehaviorGeneric, bt)

	_, err = model.ParseBehaviorType("bogus")
	require.ErrorIs(t, err, model.ErrInvalidBehaviorType)

	_, err = model.ParseBehaviorType("")
	require.ErrorIs(t, err, model.ErrInvalidBehaviorType)
}

func TestScenario_String(t *testing.T) {
	tests := []struct {
		s    model.Scenario
		want string
	}{
		{model.ScenarioSuccess, "success"},
		{model.ScenarioValidationError, "validation_error"},
		{model.ScenarioNotFound, "not_found"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.s.String(), "Scenario.String() for %v", tt.s)
	}
}

func TestScenario_IsValid(t *testing.T) {
	assert.True(t, model.ScenarioSuccess.IsValid())
	assert.True(t, model.ScenarioNotFound.IsValid())
	assert.False(t, model.Scenario("bogus").IsValid())
}

func TestBehaviorEntry_Validate(t *testing.T) {
	t.Run("valid entry", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.NoError(t, entry.Validate())
	})

	t.Run("missing method", func(t *testing.T) {
		entry := model.BehaviorEntry{
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("missing path pattern", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid behavior type", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorType("bogus"),
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("empty scenarios", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid scenario in list", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess, model.Scenario("bogus")},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("zero success code", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "POST",
			PathPattern: "/pets",
			Type:        model.BehaviorCreate,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			Source:      model.SourceHeuristic,
			Confidence:  0.8,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("invalid source", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.Source("magic"),
			Confidence:  0.9,
		}
		assert.Error(t, entry.Validate())
	})

	t.Run("confidence out of range", func(t *testing.T) {
		entry := model.BehaviorEntry{
			Method:      "GET",
			PathPattern: "/pets",
			Type:        model.BehaviorList,
			Scenarios:   []model.Scenario{model.ScenarioSuccess},
			SuccessCode: 200,
			Source:      model.SourceHeuristic,
			Confidence:  1.5,
		}
		assert.Error(t, entry.Validate())
	})
}

func TestBehaviorMap_PutAndGet(t *testing.T) {
	bm := model.NewBehaviorMap()

	entry := model.BehaviorEntry{
		Method:      "GET",
		PathPattern: "/pets",
		Type:        model.BehaviorList,
		Scenarios:   []model.Scenario{model.ScenarioSuccess},
		SuccessCode: 200,
		Source:      model.SourceHeuristic,
		Confidence:  0.9,
	}
	bm.Put(entry)

	got, ok := bm.Get("GET", "/pets")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorList, got.Type)

	_, ok = bm.Get("GET", "/nonexistent")
	assert.False(t, ok)

	_, ok = bm.Get("POST", "/pets")
	assert.False(t, ok)
}

func TestBehaviorMap_Len(t *testing.T) {
	bm := model.NewBehaviorMap()
	assert.Equal(t, 0, bm.Len())

	bm.Put(model.BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	assert.Equal(t, 1, bm.Len())

	bm.Put(model.BehaviorEntry{Method: "POST", PathPattern: "/pets"})
	assert.Equal(t, 2, bm.Len())

	// Same key overwrites, count stays the same.
	bm.Put(model.BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	assert.Equal(t, 2, bm.Len())
}

func TestBehaviorMap_Entries(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{Method: "GET", PathPattern: "/pets"})
	bm.Put(model.BehaviorEntry{Method: "POST", PathPattern: "/pets"})
	bm.Put(model.BehaviorEntry{Method: "DELETE", PathPattern: "/pets/{petId}"})

	entries := bm.Entries()
	assert.Len(t, entries, 3)
}

func TestBehaviorMapKey(t *testing.T) {
	// Same method+path should produce the same key.
	k1 := model.BehaviorMapKey("GET", "/pets")
	k2 := model.BehaviorMapKey("GET", "/pets")
	assert.Equal(t, k1, k2)

	// Different method or path should differ.
	k3 := model.BehaviorMapKey("POST", "/pets")
	assert.NotEqual(t, k1, k3)

	k4 := model.BehaviorMapKey("GET", "/pets/{petId}")
	assert.NotEqual(t, k1, k4)
}
