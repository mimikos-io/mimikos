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

// scenarioBehaviorMap builds a BehaviorMap with entries that have error
// codes and error schemas for explicit status code selection testing.
func scenarioBehaviorMap() *model.BehaviorMap {
	bm := model.NewBehaviorMap()

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,
		Scenarios:   []model.Scenario{model.ScenarioSuccess, model.ScenarioNotFound},
		SuccessCode: http.StatusOK,
		ErrorCodes:  []int{http.StatusNotFound},
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK:       {Name: "Pet", Schema: petObjectSchema()},
			http.StatusNotFound: {Name: "Error", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/pets",
		Type:        model.BehaviorCreate,
		Scenarios:   []model.Scenario{model.ScenarioSuccess, model.ScenarioValidationError},
		SuccessCode: http.StatusCreated,
		ErrorCodes:  []int{http.StatusBadRequest},
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated:    {Name: "Pet", Schema: petObjectSchema()},
			http.StatusBadRequest: {Name: "ValidationError", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets",
		Type:        model.BehaviorList,
		Scenarios:   []model.Scenario{model.ScenarioSuccess},
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "PetList", Schema: petArraySchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	// Fetch with no error schema defined — RFC 7807 fallback.
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/toys/{toyId}",
		Type:        model.BehaviorFetch,
		Scenarios:   []model.Scenario{model.ScenarioSuccess, model.ScenarioNotFound},
		SuccessCode: http.StatusOK,
		ErrorCodes:  []int{http.StatusNotFound},
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Toy", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func newScenarioTestHandler() *Handler {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)

	return NewHandler(scenarioBehaviorMap(), &stubValidator{}, resp, gen, false)
}

func TestHandler_ExplicitStatus404_WithSchema(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body, "should generate data from error schema")
}

func TestHandler_ExplicitStatus400_WithSchema(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mimikos-Status", "400")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))
}

func TestHandler_ExplicitStatus404_NoSchema_RFC7807Fallback(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/toys/42", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
	assert.Contains(t, pd["detail"], "Not Found")
}

func TestHandler_UnavailableStatusForList(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	req.Header.Set("X-Mimikos-Status", "404")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Bad Request", pd["title"])
	assert.Contains(t, pd["detail"], "404")
	assert.Contains(t, pd["detail"], "200")
}

func TestHandler_InvalidStatusNonNumeric(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
	req.Header.Set("X-Mimikos-Status", "not_a_number")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Contains(t, pd["detail"], "not_a_number")
}

func TestHandler_NoHeader_BackwardCompatible(t *testing.T) {
	h := newScenarioTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Without header, should behave exactly as before — success status code.
	assert.Equal(t, http.StatusOK, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))
}

func TestHandler_ExplicitStatus_Deterministic(t *testing.T) {
	h := newScenarioTestHandler()

	// Same request + same status should produce identical responses.
	var bodies [2]string

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
		req.Header.Set("X-Mimikos-Status", "404")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		bodies[i] = rec.Body.String()
	}

	assert.Equal(t, bodies[0], bodies[1],
		"same request + same status must produce identical responses")
}
