package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/model"
)

// --- Default scenario selection (no header) ---

func TestSelectScenario_Create(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Pet"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusCreated, result.StatusCode)
	assert.Equal(t, "Pet", result.Schema.Name)
}

func TestSelectScenario_Fetch(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "Pet", result.Schema.Name)
}

func TestSelectScenario_List(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorList,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "PetList"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "PetList", result.Schema.Name)
}

func TestSelectScenario_Update(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorUpdate,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestSelectScenario_Delete(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorDelete,
		SuccessCode: http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusNoContent: nil,
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusNoContent, result.StatusCode)
	assert.Nil(t, result.Schema)
}

func TestSelectScenario_Generic(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorGeneric,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "GenericResponse"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "GenericResponse", result.Schema.Name)
}

func TestSelectScenario_FallbackToDefaultSchema(t *testing.T) {
	// Entry has no schema for SuccessCode (200) but has default (key 0).
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			0: {Name: "DefaultResponse"},
		},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "DefaultResponse", result.Schema.Name)
}

func TestSelectScenario_NoSchemaAtAll(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:            model.BehaviorFetch,
		SuccessCode:     http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{},
	}

	result, err := SelectScenario(entry, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Nil(t, result.Schema)
}

// --- Explicit status code selection via header ---

func TestSelectScenario_Explicit404(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK:       {Name: "Pet"},
			http.StatusNotFound: {Name: "Error"},
		},
	}

	result, err := SelectScenario(entry, "404")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusNotFound, result.StatusCode)
	assert.Equal(t, "Error", result.Schema.Name)
}

func TestSelectScenario_Explicit400(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated:    {Name: "Pet"},
			http.StatusBadRequest: {Name: "ValidationError"},
		},
	}

	result, err := SelectScenario(entry, "400")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusBadRequest, result.StatusCode)
	assert.Equal(t, "ValidationError", result.Schema.Name)
}

func TestSelectScenario_Explicit422(t *testing.T) {
	// 422 Unprocessable Entity — common in GitHub API.
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated:             {Name: "Pet"},
			http.StatusUnprocessableEntity: {Name: "UnprocessableError"},
		},
	}

	result, err := SelectScenario(entry, "422")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	assert.Equal(t, "UnprocessableError", result.Schema.Name)
}

func TestSelectScenario_ExplicitSuccessCode(t *testing.T) {
	// Explicitly requesting the success code should work.
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet"},
		},
	}

	result, err := SelectScenario(entry, "200")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "Pet", result.Schema.Name)
}

func TestSelectScenario_UnavailableStatusCode(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:            model.BehaviorList,
		SuccessCode:     http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{},
	}

	result, err := SelectScenario(entry, "404")

	assert.Nil(t, result)
	require.ErrorIs(t, err, ErrStatusNotAvailable)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "200")
}

func TestSelectScenario_InvalidNonNumeric(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
	}

	result, err := SelectScenario(entry, "not_found")

	assert.Nil(t, result)
	require.ErrorIs(t, err, ErrInvalidStatusCode)
}

func TestSelectScenario_ErrorCodeNoSchema_NilFallback(t *testing.T) {
	// Spec defines 404 but no response schema — nil value in ResponseSchemas.
	// Schema should be nil — the caller handles RFC 7807 fallback.
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusNotFound: nil,
		},
	}

	result, err := SelectScenario(entry, "404")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.StatusNotFound, result.StatusCode)
	assert.Nil(t, result.Schema, "no error schema defined — caller falls back to RFC 7807")
}

func TestSelectScenario_FormatAvailableCodes(t *testing.T) {
	entry := &model.BehaviorEntry{
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK:                  {Name: "Pet"},
			http.StatusNotFound:            {Name: "Error"},
			http.StatusInternalServerError: {Name: "ServerError"},
		},
	}

	_, err := SelectScenario(entry, "422")

	require.ErrorIs(t, err, ErrStatusNotAvailable)
	// Error message should list all available codes sorted.
	assert.Contains(t, err.Error(), "200, 404, 500")
}
