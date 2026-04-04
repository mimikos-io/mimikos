package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
)

// --- helpers ---

func newStatefulGen() *generator.DataGenerator {
	return generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)
}

// newStatefulTestHandler creates a minimal Handler for stateful unit tests.
func newStatefulTestHandler(store state.Store) *Handler {
	return &Handler{
		store:  store,
		strict: false,
		logger: nil,
	}
}

func createEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/pets",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}
}

func fetchEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}
}

func listEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/pets",
		Type:        model.BehaviorList,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "PetList", Schema: petArraySchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}
}

func updateEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:      http.MethodPut,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorUpdate,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}
}

func deleteEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:          http.MethodDelete,
		PathPattern:     "/pets/{petId}",
		Type:            model.BehaviorDelete,
		SuccessCode:     http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	}
}

func genericEntry() model.BehaviorEntry {
	return model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/tasks/{taskId}/run",
		Type:        model.BehaviorGeneric,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Task", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}
}

// storeResource creates a resource directly in the store.
//
//nolint:unparam // test helper — resType varies as more test cases are added
func storeResource(t *testing.T, s state.Store, resType, id string, data map[string]any) {
	t.Helper()
	require.NoError(t, s.Put(resType, id, data))
}

// statefulMuxResult holds the captured response from a mux-dispatched stateful call.
type statefulMuxResult struct {
	recorder *httptest.ResponseRecorder
	handled  bool
}

// serveStatefulViaMux dispatches a request through a real ServeMux so that
// path parameters (r.PathValue) are populated, then delegates to handleStatefulMode.
// This is needed because handleStatefulMode relies on Go 1.22+ path value extraction.
//
//nolint:unparam // test helper — muxPattern varies across behavior types
func serveStatefulViaMux(
	t *testing.T,
	method, muxPattern, requestURL string,
	body string,
	entry model.BehaviorEntry,
	store state.Store,
) statefulMuxResult {
	t.Helper()

	h := newStatefulTestHandler(store)
	gen := newStatefulGen()
	mux := http.NewServeMux()

	var result statefulMuxResult

	mux.HandleFunc(method+" "+muxPattern, func(_ http.ResponseWriter, r *http.Request) {
		result.recorder = httptest.NewRecorder()
		result.handled = h.handleStatefulMode(result.recorder, r, entry, gen, []byte(body))
	})

	var req *http.Request

	if body != "" {
		req = httptest.NewRequest(method, requestURL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, requestURL, nil)
	}

	mux.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, result.recorder, "mux should have dispatched to handler")

	return result
}

// --- handleStatefulMode unit tests ---

func TestStateful_CreateStoresResource(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()
	entry := createEntry()

	req := httptest.NewRequest(http.MethodPost, "/pets", strings.NewReader(`{"name":"Fido"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(`{"name":"Fido"}`))

	require.True(t, handled, "create should be handled")
	assert.Equal(t, http.StatusCreated, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))

	// Response should be a valid JSON object with an id.
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "id")
	assert.Contains(t, body, "name")

	// Verify resource was stored.
	assert.Equal(t, 1, store.Count(), "store should have 1 resource")
}

func TestStateful_FetchReturnsStored(t *testing.T) {
	store := state.NewInMemory(100)
	storeResource(t, store, "pets", "42", map[string]any{"id": float64(42), "name": "Fido"})

	result := serveStatefulViaMux(t, http.MethodGet, "/pets/{petId}", "/pets/42", "", fetchEntry(), store)

	require.True(t, result.handled)
	assert.Equal(t, http.StatusOK, result.recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &body))
	assert.InDelta(t, float64(42), body["id"], 0)
	assert.Equal(t, "Fido", body["name"])
}

func TestStateful_FetchMissing404(t *testing.T) {
	store := state.NewInMemory(100)

	result := serveStatefulViaMux(t, http.MethodGet, "/pets/{petId}", "/pets/999", "", fetchEntry(), store)

	assert.Equal(t, http.StatusNotFound, result.recorder.Code)

	pd := parseProblemDetail(t, result.recorder.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
	assert.InDelta(t, float64(http.StatusNotFound), pd["status"], 0)
}

func TestStateful_ListReturnsAll(t *testing.T) {
	store := state.NewInMemory(100)
	storeResource(t, store, "pets", "1", map[string]any{"id": float64(1), "name": "Fido"})
	storeResource(t, store, "pets", "2", map[string]any{"id": float64(2), "name": "Rex"})

	h := newStatefulTestHandler(store)
	entry := listEntry()

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, newStatefulGen(), nil)

	require.True(t, handled)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body []any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body, 2)
}

func TestStateful_ListEmpty(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	entry := listEntry()

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, newStatefulGen(), nil)

	require.True(t, handled)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body []any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body)
}

func TestStateful_UpdateMergesFields(t *testing.T) {
	store := state.NewInMemory(100)
	storeResource(t, store, "pets", "42", map[string]any{
		"id": float64(42), "name": "Fido", "owner": "Alice",
	})

	result := serveStatefulViaMux(t, http.MethodPut, "/pets/{petId}", "/pets/42",
		`{"name":"Rex"}`, updateEntry(), store)

	assert.Equal(t, http.StatusOK, result.recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &body))

	// Updated field should reflect the request.
	assert.Equal(t, "Rex", body["name"])
	// Unmentioned fields should be preserved.
	assert.Equal(t, "Alice", body["owner"])
	// Server-generated fields should be preserved.
	assert.InDelta(t, float64(42), body["id"], 0)

	// Store should match the response.
	stored, found := store.Get("pets", "42")
	require.True(t, found)

	storedMap, ok := stored.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Rex", storedMap["name"])
	assert.Equal(t, "Alice", storedMap["owner"])
}

func TestStateful_UpdateMissing404(t *testing.T) {
	store := state.NewInMemory(100)

	result := serveStatefulViaMux(t, http.MethodPut, "/pets/{petId}", "/pets/999",
		`{"name":"Ghost"}`, updateEntry(), store)

	assert.Equal(t, http.StatusNotFound, result.recorder.Code)
}

func TestStateful_DeleteRemoves(t *testing.T) {
	store := state.NewInMemory(100)
	storeResource(t, store, "pets", "42", map[string]any{"id": float64(42), "name": "Fido"})

	result := serveStatefulViaMux(t, http.MethodDelete, "/pets/{petId}", "/pets/42",
		"", deleteEntry(), store)

	assert.Equal(t, http.StatusNoContent, result.recorder.Code)
	assert.Empty(t, result.recorder.Body.Bytes())

	// Verify removal.
	_, found := store.Get("pets", "42")
	assert.False(t, found, "resource should be deleted from store")
}

func TestStateful_DeleteMissing404(t *testing.T) {
	store := state.NewInMemory(100)

	result := serveStatefulViaMux(t, http.MethodDelete, "/pets/{petId}", "/pets/999",
		"", deleteEntry(), store)

	assert.Equal(t, http.StatusNotFound, result.recorder.Code)
}

func TestStateful_GenericNotHandled(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	entry := genericEntry()

	req := httptest.NewRequest(http.MethodPost, "/tasks/123/run", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, newStatefulGen(), nil)

	assert.False(t, handled, "generic behavior should not be handled by stateful logic")
}

func TestStateful_CreateNilSchema(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := model.BehaviorEntry{
		Method:          http.MethodPost,
		PathPattern:     "/items",
		Type:            model.BehaviorCreate,
		SuccessCode:     http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	}

	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(`{"name":"test"}`))

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// With nil schema, should still return a JSON body (empty object).
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
}

func TestStateful_PatchMergesFields(t *testing.T) {
	store := state.NewInMemory(100)
	storeResource(t, store, "pets", "42", map[string]any{
		"id": float64(42), "name": "Fido", "owner": "Alice",
	})

	entry := model.BehaviorEntry{
		Method:      http.MethodPatch,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorUpdate,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	result := serveStatefulViaMux(t, http.MethodPatch, "/pets/{petId}", "/pets/42",
		`{"name":"Patched"}`, entry, store)

	assert.Equal(t, http.StatusOK, result.recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &body))

	// Patched field should be updated.
	assert.Equal(t, "Patched", body["name"])
	// Unmentioned fields preserved.
	assert.Equal(t, "Alice", body["owner"])
	// Server-generated id preserved.
	assert.InDelta(t, float64(42), body["id"], 0)
}

// --- extractPathParams tests ---

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		requestURL string
		wantParams map[string]string
	}{
		{
			name:       "single param",
			pattern:    "/pets/{petId}",
			requestURL: "/pets/42",
			wantParams: map[string]string{"petId": "42"},
		},
		{
			name:       "no params",
			pattern:    "/pets",
			requestURL: "/pets",
			wantParams: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()

			var gotParams map[string]string

			mux.HandleFunc("GET "+tt.pattern, func(_ http.ResponseWriter, r *http.Request) {
				gotParams = extractPathParams(r, tt.pattern)
			})

			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)
			mux.ServeHTTP(httptest.NewRecorder(), req)

			assert.Equal(t, tt.wantParams, gotParams)
		})
	}
}
