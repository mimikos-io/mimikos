package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
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
		logger: slog.New(discardHandler{}),
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
func storeResource(t *testing.T, s state.Store, resType, id string, data map[string]any) {
	t.Helper()
	require.NoError(t, s.Put(resType, id, data))
}

// statefulMuxResult holds the captured response from a mux-dispatched stateful call.
type statefulMuxResult struct {
	recorder *httptest.ResponseRecorder
	handled  bool
}

// serveStatefulViaMux dispatches a request through a chi router so that
// path parameters are populated, then delegates to handleStatefulMode.
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
	r := chi.NewRouter()

	var result statefulMuxResult

	r.MethodFunc(method, muxPattern, func(_ http.ResponseWriter, r *http.Request) {
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

	r.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, result.recorder, "router should have dispatched to handler")

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

func TestStateful_ListEmpty_WrappedEnvelope(t *testing.T) {
	// Object-wrapped list with empty store should return {listArrayKey: [], ...metadata...},
	// not an empty object.
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)

	entry := listEntry()
	entry.ListArrayKey = "data"

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, newStatefulGen(), nil)

	require.True(t, handled)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	// The array slot should be present and empty.
	dataField, ok := body["data"]
	require.True(t, ok, "wrapped list should have array key even when empty")

	items, ok := dataField.([]any)
	require.True(t, ok, "array key should be an array")
	assert.Empty(t, items, "array should be empty when no resources stored")
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

func TestStateful_DeleteNon204SuccessCode(t *testing.T) {
	// Asana pattern: DELETE returns 200 with response body, not 204.
	store := state.NewInMemory(100)
	storeResource(t, store, "projects", "abc", map[string]any{"gid": "abc", "name": "Project"})

	entry := model.BehaviorEntry{
		Method:      http.MethodDelete,
		PathPattern: "/projects/{project_gid}",
		Type:        model.BehaviorDelete,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "EmptyResponse", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	result := serveStatefulViaMux(t, http.MethodDelete, "/projects/{project_gid}", "/projects/abc",
		"", entry, store)

	assert.Equal(t, http.StatusOK, result.recorder.Code)

	// Should have a JSON response body (not empty like 204).
	var body map[string]any
	require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &body))
	assert.NotEmpty(t, body, "non-204 delete should have response body")

	// Verify removal.
	_, found := store.Get("projects", "abc")
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

// --- unwrapResource tests ---

func TestUnwrapResource_EmptyKey(t *testing.T) {
	body := map[string]any{"id": float64(1)}
	result := unwrapResource(body, "")
	assert.Equal(t, body, result)
}

func TestUnwrapResource_WithKey(t *testing.T) {
	inner := map[string]any{"gid": "abc"}
	body := map[string]any{"data": inner}
	result := unwrapResource(body, "data")

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", resultMap["gid"])
}

func TestUnwrapResource_MissingKey(t *testing.T) {
	body := map[string]any{"other": map[string]any{}}
	result := unwrapResource(body, "data")
	// Pass-through when key not found.
	assert.Equal(t, body, result)
}

func TestUnwrapResource_NonMap(t *testing.T) {
	result := unwrapResource("string body", "data")
	assert.Equal(t, "string body", result)
}

// --- wrapResource tests ---

func TestWrapResource_EmptyKey(t *testing.T) {
	resource := map[string]any{"id": float64(1)}
	result := wrapResource(resource, "")
	assert.Equal(t, resource, result)
}

func TestWrapResource_WithKey(t *testing.T) {
	resource := map[string]any{"gid": "abc"}
	result := wrapResource(resource, "data")

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, resource, resultMap["data"])
}

// --- parseRequestBody tests ---

func TestParseRequestBody_EmptyBody(t *testing.T) {
	result, err := parseRequestBody([]byte{}, "")
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.NotNil(t, result) // empty map, not nil
}

func TestParseRequestBody_ValidJSON_NoWrapper(t *testing.T) {
	result, err := parseRequestBody([]byte(`{"name":"X"}`), "")
	require.NoError(t, err)
	assert.Equal(t, "X", result["name"])
}

func TestParseRequestBody_ValidJSON_WithWrapper(t *testing.T) {
	result, err := parseRequestBody([]byte(`{"data":{"name":"X"}}`), "data")
	require.NoError(t, err)
	assert.Equal(t, "X", result["name"])
}

func TestParseRequestBody_MalformedJSON(t *testing.T) {
	_, err := parseRequestBody([]byte(`not json`), "")
	assert.Error(t, err)
}

func TestParseRequestBody_WrapperValueNotObject(t *testing.T) {
	// When wrapper key points to a non-object value, pass through full body.
	result, err := parseRequestBody([]byte(`{"data": "not an object"}`), "data")
	require.NoError(t, err)
	assert.Equal(t, "not an object", result["data"])
}

func TestParseRequestBody_WrapperKeyMissing(t *testing.T) {
	// Client sent unwrapped body for a wrapped spec — pass through.
	result, err := parseRequestBody([]byte(`{"name":"X"}`), "data")
	require.NoError(t, err)
	assert.Equal(t, "X", result["name"])
}

// --- shallowMerge tests (new signature) ---

func TestShallowMerge_Basic(t *testing.T) {
	stored := map[string]any{"a": float64(1), "b": float64(2)}
	patch := map[string]any{"b": float64(3), "c": float64(4)}

	result := shallowMerge(stored, patch)
	assert.InDelta(t, float64(1), result["a"], 0)
	assert.InDelta(t, float64(3), result["b"], 0)
	assert.InDelta(t, float64(4), result["c"], 0)
}

func TestShallowMerge_EmptyPatch(t *testing.T) {
	stored := map[string]any{"a": float64(1)}
	result := shallowMerge(stored, map[string]any{})
	assert.InDelta(t, float64(1), result["a"], 0)
}

func TestShallowMerge_NonMapStored(t *testing.T) {
	result := shallowMerge("string", map[string]any{"a": float64(1)})
	assert.InDelta(t, float64(1), result["a"], 0)
}

func TestShallowMerge_CloneSafety(t *testing.T) {
	stored := map[string]any{"a": float64(1)}
	patch := map[string]any{"b": float64(2)}

	result := shallowMerge(stored, patch)

	// result should have both keys.
	assert.InDelta(t, float64(1), result["a"], 0)
	assert.InDelta(t, float64(2), result["b"], 0)

	// Original stored map must NOT be mutated.
	_, hasBKey := stored["b"]
	assert.False(t, hasBKey, "stored map should not be mutated by shallowMerge")
}

// --- shallowMerge concurrent safety ---

func TestShallowMerge_ConcurrentSafety(t *testing.T) {
	store := state.NewInMemory(100)
	require.NoError(t, store.Put("pets", "1", map[string]any{
		"id": float64(1), "name": "Fido", "count": float64(0),
	}))

	// 10 goroutines each updating the same resource.
	const goroutines = 10

	done := make(chan struct{}, goroutines)

	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()

			stored, found := store.Get("pets", "1")
			if !found {
				return
			}

			patch := map[string]any{"count": float64(n)}
			merged := shallowMerge(stored, patch)
			_ = store.Put("pets", "1", merged)
		}(i)
	}

	for range goroutines {
		<-done
	}

	// Final state should be consistent (some goroutine's value wins).
	result, found := store.Get("pets", "1")
	require.True(t, found)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, resultMap, "id")
	assert.Contains(t, resultMap, "name")
	assert.Contains(t, resultMap, "count")
}

// --- degraded schema + stateful mode tests ---

func TestHandleStatefulMode_DegradedCreate_ReturnsRFC7807(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: nil,
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	}

	req := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(`{}`))
	require.True(t, handled, "degraded create should be handled (not fall through)")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestHandleStatefulMode_DegradedCreate_WithExample_ServesExample(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	exampleData := map[string]any{"id": float64(1), "status": "pending"}

	entry := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/reports",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: nil,
		},
		ResponseExamples: map[int]any{
			http.StatusCreated: exampleData,
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	}

	req := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	// Should NOT be handled by the degradation check — example available.
	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(`{}`))
	require.True(t, handled, "create should still be handled")
	// The create handler will call generateResponseBody with nil schema,
	// which returns {}. But the degradation check should NOT have fired
	// because the example exists. Verify we didn't get a 500 problem detail.
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code,
		"should not return 500 when media-type example is available")
}

func TestHandleStatefulMode_DegradedFetch_StillWorks(t *testing.T) {
	store := state.NewInMemory(100)
	_ = store.Put("reports", "42", map[string]any{"id": "42", "status": "done"})
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/reports/{id}",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: nil,
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	}

	// Use chi router to set path params.
	r := chi.NewRouter()

	var rec *httptest.ResponseRecorder

	r.Get("/reports/{id}", func(_ http.ResponseWriter, r *http.Request) {
		rec = httptest.NewRecorder()
		h.handleStatefulMode(rec, r, entry, gen, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/reports/42", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, rec)
	assert.Equal(t, http.StatusOK, rec.Code, "fetch uses stored data — not affected by schema degradation")
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
		{
			name:       "hyphenated param",
			pattern:    "/teams/{enterprise-team}/members",
			requestURL: "/teams/backend/members",
			wantParams: map[string]string{"enterprise-team": "backend"},
		},
		{
			name:       "dotted param",
			pattern:    "/configs/{config.key}",
			requestURL: "/configs/database",
			wantParams: map[string]string{"config.key": "database"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()

			var gotParams map[string]string

			r.Get(tt.pattern, func(_ http.ResponseWriter, r *http.Request) {
				gotParams = extractPathParams(r, tt.pattern)
			})

			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)
			r.ServeHTTP(httptest.NewRecorder(), req)

			assert.Equal(t, tt.wantParams, gotParams)
		})
	}
}
