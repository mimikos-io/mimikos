package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/model"
)

func TestSelectScenario_Create(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorCreate,
		SuccessCode: 201,
		ResponseSchemas: map[int]*model.CompiledSchema{
			201: {Name: "Pet"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 201, result.StatusCode)
	assert.Equal(t, "Pet", result.Schema.Name)
}

func TestSelectScenario_Fetch(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: {Name: "Pet"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "Pet", result.Schema.Name)
}

func TestSelectScenario_List(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorList,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: {Name: "PetList"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "PetList", result.Schema.Name)
}

func TestSelectScenario_Update(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorUpdate,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: {Name: "Pet"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 200, result.StatusCode)
}

func TestSelectScenario_Delete(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorDelete,
		SuccessCode: 204,
		ResponseSchemas: map[int]*model.CompiledSchema{
			204: nil,
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 204, result.StatusCode)
	assert.Nil(t, result.Schema)
}

func TestSelectScenario_Generic(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorGeneric,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: {Name: "GenericResponse"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, model.ScenarioSuccess, result.Scenario)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "GenericResponse", result.Schema.Name)
}

func TestSelectScenario_FallbackToDefaultSchema(t *testing.T) {
	// Entry has no schema for SuccessCode (200) but has default (key 0).
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			0: {Name: "DefaultResponse"},
		},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "DefaultResponse", result.Schema.Name)
}

func TestSelectScenario_NoSchemaAtAll(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:            model.BehaviorFetch,
		SuccessCode:     200,
		ResponseSchemas: map[int]*model.CompiledSchema{},
	}

	result := SelectScenario(entry)

	require.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
	assert.Nil(t, result.Schema)
}
