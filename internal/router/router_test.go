package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/builder"
	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/validator"
)

// --- test helpers ---

// stubValidator implements validator.RequestValidator for test control.
type stubValidator struct {
	errs []model.ValidationError
	err  error
}

func (s *stubValidator) Validate(*http.Request) ([]model.ValidationError, error) {
	return s.errs, s.err
}

// newTestTypes creates a *jsonschema.Types from type name strings.
func newTestTypes(types ...string) *jsonschema.Types {
	var tt jsonschema.Types
	for _, t := range types {
		tt.Add(t)
	}

	return &tt
}

// petObjectSchema returns a simple object schema with id (integer) and name (string).
func petObjectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Types: newTestTypes("object"),
		Properties: map[string]*jsonschema.Schema{
			"id":   {Types: newTestTypes("integer")},
			"name": {Types: newTestTypes("string")},
		},
	}
}

// petArraySchema returns an array schema with petObjectSchema items.
func petArraySchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Types:     newTestTypes("array"),
		Items2020: petObjectSchema(),
	}
}

// testBehaviorMap builds a BehaviorMap with pet CRUD + nested toy fetch.
func testBehaviorMap() *model.BehaviorMap {
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
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,

		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
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
			http.StatusCreated: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodDelete,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorDelete,

		SuccessCode:     http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}/toys/{toyId}",
		Type:        model.BehaviorFetch,

		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Toy", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func newTestHandler(v validator.RequestValidator) *Handler {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	return NewHandler(testBehaviorMap(), nil, v, resp, gen, false, nil, model.ModeDeterministic, nil)
}

// parseProblemDetail decodes an RFC 7807 response body.
func parseProblemDetail(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var pd map[string]any

	require.NoError(t, json.Unmarshal(body, &pd))

	return pd
}

// --- Route matching tests ---

func TestHandler_GetCollection(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body []any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body, "list response should be a non-empty array")
}

func TestHandler_GetItem(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id")
	assert.Contains(t, body, "name")
}

func TestHandler_PostCreate(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id")
}

func TestHandler_Delete(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodDelete, "/pets/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes(), "204 should have no body")
}

func TestHandler_NestedParams(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets/123/toys/456", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id")
}

// --- Error cases ---

func TestHandler_NotFound(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
	assert.InDelta(t, float64(http.StatusNotFound), pd["status"], 0)
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPatch, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Allow"), "405 should have Allow header")

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Method Not Allowed", pd["title"])
}

// --- Content-type and validation tests ---

func TestHandler_UnsupportedContentType(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`<pet><name>Fido</name></pet>`))
	req.Header.Set("Content-Type", "text/xml")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Unsupported Media Type", pd["title"])
}

func TestHandler_ValidationErrors(t *testing.T) {
	v := &stubValidator{
		errs: []model.ValidationError{
			{Field: "/name", Message: "required field missing"},
		},
	}
	h := newTestHandler(v)

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Bad Request", pd["title"])
	assert.NotNil(t, pd["errors"], "should contain validation errors")
}

func TestHandler_NoBodyNoContentTypeIsValid(t *testing.T) {
	// POST with no body and no Content-Type should not trigger 415.
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Should reach success path (not 415).
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_GetIgnoresContentType(t *testing.T) {
	// GET with Content-Type header should be ignored (some clients send it).
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	req.Header.Set("Content-Type", "text/xml")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_JsonContentTypeVariant(t *testing.T) {
	// application/vnd.api+json should be accepted as JSON.
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_BodySizeLimit(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	// Create a body larger than maxBodySize (10MB + 1 byte).
	largeBody := strings.NewReader(strings.Repeat("x", 10*1024*1024+1))
	req := httptest.NewRequest(http.MethodPost, "/pets", largeBody)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Request Entity Too Large", pd["title"])
}

// --- Deterministic response test ---

func TestHandler_DeterministicResponse(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	// Same request twice should produce identical responses.
	var bodies [2]string

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		bodies[i] = rec.Body.String()
	}

	assert.Equal(t, bodies[0], bodies[1], "identical requests must produce identical responses")
}

func TestHandler_DifferentPathParamsDifferentResponse(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req1 := httptest.NewRequest(http.MethodGet, "/pets/1", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/pets/2", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	assert.NotEqual(t, rec1.Body.String(), rec2.Body.String(),
		"different path params should produce different responses")
}

// --- Validator sentinel error tests (NB5) ---

func TestHandler_ValidatorPathNotFound(t *testing.T) {
	v := &stubValidator{err: validator.ErrPathNotFound}
	h := newTestHandler(v)

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
}

func TestHandler_ValidatorOperationNotFound(t *testing.T) {
	v := &stubValidator{err: validator.ErrOperationNotFound}
	h := newTestHandler(v)

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Method Not Allowed", pd["title"])
}

// --- Nil schema response test (NB6) ---

func TestHandler_NilSchemaReturnsEmptyObject(t *testing.T) {
	// Build a behavior map with a fetch entry that has no response schema.
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/empty",
		Type:        model.BehaviorFetch,

		SuccessCode:     http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body, "nil schema should produce empty object {}")
}

// --- Literal/wildcard sibling route conflict tests ---

// conflictingPathsBehaviorMap creates a BehaviorMap where a literal path
// (/items/shared) is a sibling of a wildcard path (/items/{id}).
// Kept as a regression guard for correct literal-vs-wildcard precedence.
func conflictingPathsBehaviorMap() *model.BehaviorMap {
	bm := model.NewBehaviorMap()

	// Wildcard: PUT /items/{id}
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPut,
		PathPattern: "/items/{id}",
		Type:        model.BehaviorUpdate,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Item", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	// Wildcard: DELETE /items/{id}
	bm.Put(model.BehaviorEntry{
		Method:          http.MethodDelete,
		PathPattern:     "/items/{id}",
		Type:            model.BehaviorDelete,
		SuccessCode:     http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	})

	// Literal sibling: GET /items/shared
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/items/shared",
		Type:        model.BehaviorList,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "SharedItems", Schema: petArraySchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func newConflictingHandler() *Handler {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	bm := conflictingPathsBehaviorMap()

	return NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)
}

func TestHandler_LiteralWildcardSibling_NoPanic(t *testing.T) {
	// Regression guard: literal/wildcard sibling routes must register without panic.
	assert.NotPanics(t, func() {
		newConflictingHandler()
	})
}

func TestHandler_LiteralWildcardSibling_OperationRoutes(t *testing.T) {
	h := newConflictingHandler()

	t.Run("GET /items/shared returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items/shared", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("PUT /items/123 returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/items/123",
			strings.NewReader(`{"name":"updated"}`))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("DELETE /items/123 returns 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/items/123", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestHandler_LiteralWildcardSibling_MethodNotAllowed(t *testing.T) {
	h := newConflictingHandler()

	t.Run("POST /items/shared returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/items/shared", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		pd := parseProblemDetail(t, rec.Body.Bytes())
		assert.Equal(t, "Method Not Allowed", pd["title"])
	})

	t.Run("GET /items/123 returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items/123", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		pd := parseProblemDetail(t, rec.Body.Bytes())
		assert.Equal(t, "Method Not Allowed", pd["title"])
	})
}

// --- Media-type example response tests (Task 25) ---

func TestHandler_ExampleResponse_ReturnsExampleBody(t *testing.T) {
	example := map[string]any{"id": float64(42), "name": "Fido", "tag": "dog"}

	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
		},
		ResponseExamples: map[int]any{
			http.StatusOK: example,
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodGet, "/pets/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.InDelta(t, 42.0, body["id"], 0.1)
	assert.Equal(t, "Fido", body["name"])
	assert.Equal(t, "dog", body["tag"])
}

func TestHandler_NoExample_FallsThroughToGeneration(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
		},
		// No ResponseExamples — should use generation as before.
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodGet, "/pets/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id", "generated response should have id")
	assert.Contains(t, body, "name", "generated response should have name")
}

func TestHandler_ExampleResponse_204NoContent_NeverReturnsBody(t *testing.T) {
	// Even if an example exists for 204, no body should be written.
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodDelete,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorDelete,
		SuccessCode: http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusNoContent: nil,
		},
		ResponseExamples: map[int]any{
			http.StatusNoContent: map[string]any{"message": "deleted"},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodDelete, "/pets/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes(), "204 should have no body even with example")
}

// --- Required body validation tests (Task 26) ---

func TestHandler_RequiredBodyMissing_Returns400(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:       http.MethodPost,
		PathPattern:  "/pets",
		Type:         model.BehaviorCreate,
		SuccessCode:  http.StatusCreated,
		BodyRequired: true,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Bad Request", pd["title"])

	errors, ok := pd["errors"].([]any)
	require.True(t, ok, "should contain errors array")
	require.Len(t, errors, 1)

	firstErr, ok := errors[0].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, firstErr["field"])
	assert.Equal(t, "Request body is required", firstErr["message"])
}

func TestHandler_RequiredBodyPresent_Succeeds(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:       http.MethodPost,
		PathPattern:  "/pets",
		Type:         model.BehaviorCreate,
		SuccessCode:  http.StatusCreated,
		BodyRequired: true,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_OptionalBodyMissing_Succeeds(t *testing.T) {
	// POST with no body when BodyRequired is false should pass through.
	// (testBehaviorMap defaults BodyRequired to false.)
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

// --- Panic recovery middleware tests (Task 27) ---

func TestHandler_Recoverer_PanicReturnsRFC7807(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/panic",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Test", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	// Build handler, then replace the route with one that panics.
	// We can't inject a panicking handler through NewHandler directly,
	// so we use chi's route replacement by registering after construction.
	// Instead, test the middleware by creating a handler that will panic
	// at request time due to a nil schema dereference path.
	// Actually, let's test the recoverer directly via the middleware chain.
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	// Override the route with a panicking handler to test the middleware.
	h.router.Get("/boom", func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic in handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()

	// Must not panic — recoverer catches it.
	assert.NotPanics(t, func() {
		h.ServeHTTP(rec, req)
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Internal Server Error", pd["title"])
	assert.Contains(t, pd["detail"], "test panic in handler")
	assert.Contains(t, pd["detail"], "GET /boom")
}

func TestHandler_Recoverer_NormalRequestUnaffected(t *testing.T) {
	h := newTestHandler(&stubValidator{})

	req := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))
}

// --- Failed entry placeholder tests (Task 27) ---

func TestHandler_FailedEntry_ReturnsRFC7807(t *testing.T) {
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

	failed := []builder.FailedEntry{
		{Method: http.MethodGet, PathPattern: "/dogs/{dogId}", Error: "nil pointer dereference"},
	}

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, failed, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	// Request to failed endpoint → actionable 500.
	req := httptest.NewRequest(http.MethodGet, "/dogs/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Internal Server Error", pd["title"])
	assert.Contains(t, pd["detail"], "failed to register at startup")
	assert.Contains(t, pd["detail"], "nil pointer dereference")
}

func TestHandler_FailedEntry_SuccessfulRoutesUnaffected(t *testing.T) {
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

	failed := []builder.FailedEntry{
		{Method: http.MethodGet, PathPattern: "/dogs/{dogId}", Error: "nil pointer dereference"},
	}

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, failed, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	// Request to successful endpoint → normal response.
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- isJSONContentType unit tests (NB2) ---

func TestIsJSONContentType(t *testing.T) {
	tests := []struct {
		name string
		ct   string
		want bool
	}{
		{name: "exact json", ct: "application/json", want: true},
		{name: "json with charset", ct: "application/json; charset=utf-8", want: true},
		{name: "json uppercase", ct: "Application/JSON", want: true},
		{name: "vnd+json", ct: "application/vnd.api+json", want: true},
		{name: "merge-patch+json", ct: "application/merge-patch+json", want: true},
		{name: "xml", ct: "text/xml", want: false},
		{name: "text/json", ct: "text/json", want: false},
		{name: "plain text", ct: "text/plain", want: false},
		{name: "empty", ct: "", want: false},
		{name: "form urlencoded", ct: "application/x-www-form-urlencoded", want: false},
		{name: "multipart", ct: "multipart/form-data", want: false},
		{name: "json with spaces", ct: " application/json ", want: true},
		{name: "degenerate +json", ct: "application/+json", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isJSONContentType(tt.ct))
		})
	}
}

// --- Degraded schema tests ---

func TestHandler_DegradedResponseSchema_ReturnsRFC7807(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: nil, // schema failed to compile
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed: 'examples' got object, want array",
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/reports",
		strings.NewReader(`{"startDate": "2024-01-01"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.InDelta(t, http.StatusInternalServerError, pd["status"], 0)
	assert.Contains(t, pd["detail"], "schema failed to compile")
}

func TestHandler_DegradedResponseSchema_WithExample_ServesExample(t *testing.T) {
	// When a degraded endpoint has a media-type example, serve it instead
	// of returning RFC 7807. The example doesn't need a compiled schema.
	exampleData := map[string]any{"id": float64(1), "status": "pending"}

	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: nil, // schema failed
		},
		ResponseExamples: map[int]any{
			http.StatusCreated: exampleData,
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/reports",
		strings.NewReader(`{"startDate": "2024-01-01"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Should serve the example, not RFC 7807.
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "pending", body["status"])
}

func TestHandler_DegradedRequestSchema_StrictMode_ReturnsRFC7807(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Report", Schema: petObjectSchema()},
		},
		Source:                model.SourceHeuristic,
		Confidence:            0.9,
		DegradedRequestSchema: "compiler: schema compilation failed",
		BodyRequired:          true,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	// strict=true
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, true, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/reports",
		strings.NewReader(`{"startDate": "2024-01-01"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Contains(t, pd["detail"], "schema failed to compile")
}

func TestHandler_DegradedRequestSchema_NonStrict_Works(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Report", Schema: petObjectSchema()},
		},
		Source:                model.SourceHeuristic,
		Confidence:            0.9,
		DegradedRequestSchema: "compiler: schema compilation failed",
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	// strict=false
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/reports",
		strings.NewReader(`{"startDate": "2024-01-01"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Should work normally — non-strict skips the degradation check.
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestHandler_DegradedRequestSchema_StrictMode_GET_Works(t *testing.T) {
	// GET has no body — request schema degradation should not affect it.
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/reports",
		Type:        model.BehaviorList,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "ReportList", Schema: petArraySchema()},
		},
		Source:                model.SourceHeuristic,
		Confidence:            0.9,
		DegradedRequestSchema: "compiler: schema compilation failed",
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	// strict=true, but GET has no body.
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, true, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_DegradedResponseSchema_NonSuccessCode_Works(t *testing.T) {
	// Degradation is tracked for the success code. Requesting a non-degraded
	// error code via X-Mimikos-Status should still work.
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated:    nil, // degraded
			http.StatusBadRequest: nil, // error fallback, not degraded
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
	h := NewHandler(bm, nil, &stubValidator{}, resp, gen, false, nil, model.ModeDeterministic, nil)

	req := httptest.NewRequest(http.MethodPost, "/reports",
		strings.NewReader(`{"startDate": "2024-01-01"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mimikos-Status", "400")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Should return the error fallback, not the degradation error.
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
