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

	return NewHandler(testBehaviorMap(), v, resp, gen, false, nil)
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
	h := NewHandler(bm, &stubValidator{}, resp, gen, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]any

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body, "nil schema should produce empty object {}")
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
