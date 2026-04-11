package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
)

// Integration tests for the full runtime pipeline:
//   Request → Router → RequestValidator → (Error Responder | Fingerprint →
//   ScenarioRouter → DataGenerator → ResponseValidator) → Response

// pipelineBehaviorMap creates a BehaviorMap that exercises the full pipeline
// with all behavior types, scenarios, and error codes.
func pipelineBehaviorMap() *model.BehaviorMap {
	bm := model.NewBehaviorMap()

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets",
		Type:        model.BehaviorList,

		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "PetList", Schema: petArraySchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/pets",
		Type:        model.BehaviorCreate,

		SuccessCode: http.StatusCreated,

		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated:    {Name: "Pet", Schema: petObjectSchema()},
			http.StatusBadRequest: {Name: "Error", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,

		SuccessCode: http.StatusOK,

		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK:       {Name: "Pet", Schema: petObjectSchema()},
			http.StatusNotFound: {Name: "Error", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodDelete,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorDelete,
		SuccessCode: http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusNotFound: nil, // defined in spec but no schema
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func newPipelineHandler(v *stubValidator) *Handler {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	return NewHandler(pipelineBehaviorMap(), nil, v, resp, gen, false, nil, model.ModeDeterministic, nil)
}

// --- Pipeline integration tests ---

func TestPipeline_GetCollection(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body []any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body, "list response should be a non-empty array")

	// Each item should be an object with id and name.
	first, ok := body[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, first, "id")
	assert.Contains(t, first, "name")
}

func TestPipeline_PostCreate(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id")
	assert.Contains(t, body, "name")
}

func TestPipeline_PostInvalid_ValidationError(t *testing.T) {
	v := &stubValidator{
		errs: []model.ValidationError{
			{Field: "/name", Message: "required field missing"},
		},
	}
	h := newPipelineHandler(v)

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Bad Request", pd["title"])
	assert.Contains(t, pd["detail"], "validation")

	errors, ok := pd["errors"].([]any)
	require.True(t, ok)
	assert.Len(t, errors, 1)
}

func TestPipeline_UnknownRoute_404(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
}

func TestPipeline_MethodNotAllowed_405(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPatch, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Allow"))
}

func TestPipeline_Delete_204(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodDelete, "/pets/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}

func TestPipeline_ExplicitStatus_404(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPipeline_ExplicitStatus_Unavailable(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	// list behavior has no 404 defined.
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPipeline_Delete404_RFC7807Fallback(t *testing.T) {
	h := newPipelineHandler(&stubValidator{})

	// Delete has no error schema — should get RFC 7807 fallback.
	req := httptest.NewRequest(http.MethodDelete, "/pets/123", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
	assert.Contains(t, pd["detail"], "Not Found")
}

// --- Benchmark ---

func BenchmarkPipeline_GetItem(b *testing.B) {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(pipelineBehaviorMap(), nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}

func BenchmarkPipeline_PostCreate(b *testing.B) {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(pipelineBehaviorMap(), nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	body := `{"name":"Fido"}`

	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/pets",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}

func BenchmarkPipeline_GetCollection(b *testing.B) {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(pipelineBehaviorMap(), nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/pets", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}
