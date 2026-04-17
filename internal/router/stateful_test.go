package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	require.NoError(t, s.Put(resType, "", id, data))
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
	stored, found := store.Get("pets", "", "42")
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
	_, found := store.Get("pets", "", "42")
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
	_, found := store.Get("projects", "", "abc")
	assert.False(t, found, "resource should be deleted from store")
}

func TestStateful_DeleteMissing404(t *testing.T) {
	store := state.NewInMemory(100)

	result := serveStatefulViaMux(t, http.MethodDelete, "/pets/{petId}", "/pets/999",
		"", deleteEntry(), store)

	assert.Equal(t, http.StatusNotFound, result.recorder.Code)
}

// --- Nested path scope isolation (unit-level) ---

func TestStateful_NestedCreate_ScopesByParent(t *testing.T) {
	store := state.NewInMemory(100)

	nestedCreate := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/orgs/{orgId}/teams",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Team", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	// Create a team under org1.
	result1 := serveStatefulViaMux(t, http.MethodPost, "/orgs/{orgId}/teams",
		"/orgs/org1/teams", `{"name":"Alpha"}`, nestedCreate, store)

	require.True(t, result1.handled)
	assert.Equal(t, http.StatusCreated, result1.recorder.Code)
	assert.Equal(t, 1, store.Count())

	// Create a team under org2.
	result2 := serveStatefulViaMux(t, http.MethodPost, "/orgs/{orgId}/teams",
		"/orgs/org2/teams", `{"name":"Beta"}`, nestedCreate, store)

	require.True(t, result2.handled)
	assert.Equal(t, http.StatusCreated, result2.recorder.Code)
	assert.Equal(t, 2, store.Count())

	// List org1 — should see only 1 item.
	nestedList := model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/orgs/{orgId}/teams",
		Type:        model.BehaviorList,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "TeamList", Schema: petArraySchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	listResult := serveStatefulViaMux(t, http.MethodGet, "/orgs/{orgId}/teams",
		"/orgs/org1/teams", "", nestedList, store)

	require.True(t, listResult.handled)
	assert.Equal(t, http.StatusOK, listResult.recorder.Code)

	var items []any
	require.NoError(t, json.Unmarshal(listResult.recorder.Body.Bytes(), &items))
	assert.Len(t, items, 1, "org1 scope should have exactly 1 team, not 2")
}

func TestStateful_NestedFetch_WrongScope404(t *testing.T) {
	store := state.NewInMemory(100)

	nestedCreate := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/orgs/{orgId}/teams",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Team", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	// Create under org1.
	result := serveStatefulViaMux(t, http.MethodPost, "/orgs/{orgId}/teams",
		"/orgs/org1/teams", `{"name":"Alpha"}`, nestedCreate, store)

	require.True(t, result.handled)

	var created map[string]any
	require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &created))
	require.Contains(t, created, "id")

	id := created["id"]

	var idStr string

	switch v := id.(type) {
	case float64:
		idStr = strconv.FormatInt(int64(v), 10)
	case string:
		idStr = v
	default:
		t.Fatalf("unexpected id type: %T", id)
	}

	nestedFetch := model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/orgs/{orgId}/teams/{teamId}",
		Type:        model.BehaviorFetch,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Team", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	// Fetch under org2 — should 404 (resource is in org1's scope).
	fetchResult := serveStatefulViaMux(t, http.MethodGet, "/orgs/{orgId}/teams/{teamId}",
		"/orgs/org2/teams/"+idStr, "", nestedFetch, store)

	require.True(t, fetchResult.handled)
	assert.Equal(t, http.StatusNotFound, fetchResult.recorder.Code,
		"fetch under wrong parent scope should return 404")
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
	require.NoError(t, store.Put("pets", "", "1", map[string]any{
		"id": float64(1), "name": "Fido", "count": float64(0),
	}))

	// 10 goroutines each updating the same resource.
	const goroutines = 10

	done := make(chan struct{}, goroutines)

	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()

			stored, found := store.Get("pets", "", "1")
			if !found {
				return
			}

			patch := map[string]any{"count": float64(n)}
			merged := shallowMerge(stored, patch)
			_ = store.Put("pets", "", "1", merged)
		}(i)
	}

	for range goroutines {
		<-done
	}

	// Final state should be consistent (some goroutine's value wins).
	result, found := store.Get("pets", "", "1")
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
	_ = store.Put("reports", "", "42", map[string]any{"id": "42", "status": "done"})
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

// --- buildIDFieldSet tests ---

func TestBuildIDFieldSet_FlatPath(t *testing.T) {
	entry := model.BehaviorEntry{
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorCreate,
	}

	set := buildIDFieldSet(entry)

	// "id" is always present (strategy 4 universal fallback).
	assert.True(t, set["id"], "should always include 'id'")
	// "petId" from lastPathParam (strategy 1).
	assert.True(t, set["petid"], "should include lowercased lastPathParam")
}

func TestBuildIDFieldSet_CollectionPath_NoLastParam(t *testing.T) {
	// POST /pets has no path param — only "id" + IDFieldHint.
	entry := model.BehaviorEntry{
		PathPattern: "/pets",
		Type:        model.BehaviorCreate,
		IDFieldHint: "gid",
	}

	set := buildIDFieldSet(entry)

	assert.True(t, set["id"], "should always include 'id'")
	assert.True(t, set["gid"], "should include IDFieldHint")
	assert.Len(t, set, 2)
}

func TestBuildIDFieldSet_NestedPath(t *testing.T) {
	entry := model.BehaviorEntry{
		PathPattern: "/projects/{project_gid}/tasks/{task_gid}",
		Type:        model.BehaviorCreate,
		IDFieldHint: "gid",
	}

	set := buildIDFieldSet(entry)

	assert.True(t, set["id"])
	assert.True(t, set["task_gid"], "should include lowercased lastPathParam")
	assert.True(t, set["gid"], "should include stripped remainder and IDFieldHint")
}

func TestBuildIDFieldSet_AlwaysIncludesID(t *testing.T) {
	entry := model.BehaviorEntry{
		PathPattern: "/items",
		Type:        model.BehaviorCreate,
	}

	set := buildIDFieldSet(entry)
	assert.True(t, set["id"], "should always include 'id' even with no params")
}

// --- shallowMergeExcluding tests ---

func TestShallowMergeExcluding_Basic(t *testing.T) {
	base := map[string]any{"id": float64(42), "name": "Faker", "status": "active"}
	overlay := map[string]any{"id": float64(1), "name": "Example", "color": "blue"}
	exclude := map[string]bool{"id": true}

	result := shallowMergeExcluding(base, overlay, exclude)

	// "id" should be preserved from base (protected).
	assert.InDelta(t, float64(42), result["id"], 0, "id should be protected from overlay")
	// "name" should come from overlay (not protected).
	assert.Equal(t, "Example", result["name"])
	// "color" should be added from overlay.
	assert.Equal(t, "blue", result["color"])
	// "status" preserved from base.
	assert.Equal(t, "active", result["status"])
}

func TestShallowMergeExcluding_CaseInsensitive(t *testing.T) {
	base := map[string]any{"ID": float64(42), "name": "Faker"}
	overlay := map[string]any{"ID": float64(1), "name": "Example"}
	exclude := map[string]bool{"id": true} // lowercase in set

	result := shallowMergeExcluding(base, overlay, exclude)

	assert.InDelta(t, float64(42), result["ID"], 0, "case-insensitive protection")
	assert.Equal(t, "Example", result["name"])
}

func TestShallowMergeExcluding_EmptyExclude(t *testing.T) {
	base := map[string]any{"id": float64(42)}
	overlay := map[string]any{"id": float64(1), "name": "Example"}

	result := shallowMergeExcluding(base, overlay, map[string]bool{})

	// With empty exclude set, behaves like normal shallowMerge.
	assert.InDelta(t, float64(1), result["id"], 0)
	assert.Equal(t, "Example", result["name"])
}

func TestShallowMergeExcluding_EmptyOverlay(t *testing.T) {
	base := map[string]any{"id": float64(42), "name": "Faker"}

	result := shallowMergeExcluding(base, map[string]any{}, map[string]bool{"id": true})

	assert.InDelta(t, float64(42), result["id"], 0)
	assert.Equal(t, "Faker", result["name"])
}

func TestShallowMergeExcluding_CloneSafety(t *testing.T) {
	base := map[string]any{"id": float64(42), "name": "Faker"}
	overlay := map[string]any{"name": "Example"}

	result := shallowMergeExcluding(base, overlay, map[string]bool{"id": true})

	// Mutating the result should not affect the original.
	result["extra"] = "added"
	_, hasExtra := base["extra"]
	assert.False(t, hasExtra, "base map should not be mutated")
}

func TestShallowMergeExcluding_MultipleExcludedFields(t *testing.T) {
	base := map[string]any{"id": float64(42), "gid": "abc", "name": "Faker"}
	overlay := map[string]any{"id": float64(1), "gid": "example-gid", "name": "Example"}
	exclude := map[string]bool{"id": true, "gid": true}

	result := shallowMergeExcluding(base, overlay, exclude)

	assert.InDelta(t, float64(42), result["id"], 0, "id protected")
	assert.Equal(t, "abc", result["gid"], "gid protected")
	assert.Equal(t, "Example", result["name"], "name not protected")
}

// --- handleCreate merge behavior tests ---

func TestHandleCreate_MergesRequestBody(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()
	entry := createEntry()

	body := `{"name":"Buddy","tag":"dog"}`
	req := httptest.NewRequest(http.MethodPost, "/pets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(body))

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Request body values should override generated values.
	assert.Equal(t, "Buddy", resp["name"], "request body name should override generated")
	assert.Equal(t, "dog", resp["tag"], "request body tag should override generated")
	// Generated id should still be present.
	assert.Contains(t, resp, "id", "generated id should be present")
}

func TestHandleCreate_EmptyBody(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()
	entry := createEntry()

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte{})

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Should be fully generated — no merge, no error.
	assert.Contains(t, resp, "id")
	assert.Contains(t, resp, "name")
}

func TestHandleCreate_WithExample_IDProtected(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := createEntry()
	entry.ResponseExamples = map[int]any{
		http.StatusCreated: map[string]any{
			"id":   float64(1),
			"name": "Example Pet",
			"tag":  "example-tag",
		},
	}

	body := `{"name":"Buddy"}`
	req := httptest.NewRequest(http.MethodPost, "/pets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(body))

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Request body wins over example.
	assert.Equal(t, "Buddy", resp["name"], "request body should override example")
	// Example's "tag" should be used (not in request body, better than faker).
	assert.Equal(t, "example-tag", resp["tag"], "example tag should override faker")
	// ID should NOT be from example (protected) — should be faker-generated.
	assert.NotEqual(t, float64(1), resp["id"], "example id should be protected; faker id used")
	assert.Contains(t, resp, "id")
}

func TestHandleCreate_ExampleOnly_NoRequestBody(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := createEntry()
	entry.ResponseExamples = map[int]any{
		http.StatusCreated: map[string]any{
			"id":   float64(1),
			"name": "Example Pet",
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte{})

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Example name should override faker name.
	assert.Equal(t, "Example Pet", resp["name"], "example name should override faker")
	// ID should NOT be from example.
	assert.NotEqual(t, float64(1), resp["id"], "example id should be protected")
	assert.Contains(t, resp, "id")
}

func TestHandleCreate_MultipleCreates_UniqueIDs(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	entry := createEntry()
	entry.ResponseExamples = map[int]any{
		http.StatusCreated: map[string]any{
			"id":   float64(1),
			"name": "Example Pet",
		},
	}

	// First create.
	body1 := `{"name":"First"}`
	req1 := httptest.NewRequest(http.MethodPost, "/pets", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")

	rec1 := httptest.NewRecorder()

	h.handleStatefulMode(rec1, req1, entry, gen, []byte(body1))

	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	// Second create with different body.
	body2 := `{"name":"Second"}`
	req2 := httptest.NewRequest(http.MethodPost, "/pets", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")

	rec2 := httptest.NewRecorder()

	h.handleStatefulMode(rec2, req2, entry, gen, []byte(body2))

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

	// Both should have IDs, and they should differ.
	require.Contains(t, resp1, "id")
	require.Contains(t, resp2, "id")
	assert.NotEqual(t, resp1["id"], resp2["id"],
		"two creates with same example should produce unique IDs")

	// Store should have 2 resources.
	assert.Equal(t, 2, store.Count(), "store should have 2 resources")
}

func TestHandleCreate_WrappedResource_MergesAllLayers(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	// Wrapped entry (Asana-style): response wrapped in {"data": {...}}.
	entry := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/projects",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		WrapperKey:  "data",
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: {Name: "Project", Schema: petObjectSchema()},
		},
		ResponseExamples: map[int]any{
			http.StatusCreated: map[string]any{
				"data": map[string]any{
					"id":    float64(99),
					"name":  "Example Project",
					"color": "blue",
				},
			},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	}

	body := `{"data":{"name":"My Project"}}`
	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(body))

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Response should be wrapped.
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "response should have 'data' wrapper")

	// Request body wins for name.
	assert.Equal(t, "My Project", data["name"], "request body overrides example")
	// Example provides color (not in request body, not in faker schema).
	assert.Equal(t, "blue", data["color"], "example color should override faker")
	// ID should NOT be from example (protected).
	assert.NotEqual(t, float64(99), data["id"], "example id should be protected")
	assert.Contains(t, data, "id")
}

// --- degraded create + example merge tests ---

func TestHandleCreate_DegradedWithExample_MergesRequestBody(t *testing.T) {
	// Degraded path: schema is nil but example exists.
	// Example provides the base (including ID — not protected when degraded).
	// Request body overrides example fields.
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	exampleData := map[string]any{
		"id":     float64(42),
		"name":   "Example Name",
		"status": "active",
	}

	entry := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/items",
		Type:        model.BehaviorCreate,
		SuccessCode: http.StatusCreated,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusCreated: nil, // degraded — no compiled schema
		},
		ResponseExamples: map[int]any{
			http.StatusCreated: exampleData,
		},
		Source:                 model.SourceHeuristic,
		Confidence:             0.9,
		DegradedResponseSchema: "compiler: schema compilation failed",
	}

	body := `{"name":"Custom Name"}`
	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte(body))

	require.True(t, handled)
	// Should NOT be 500 — example provides a fallback.
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Example ID is NOT protected when degraded — a static ID is better than none.
	assert.InDelta(t, float64(42), resp["id"], 0,
		"degraded path should include example ID (not protected)")
	// Request body overrides example.
	assert.Equal(t, "Custom Name", resp["name"],
		"request body should override example name")
	// Example field not in request body should be present.
	assert.Equal(t, "active", resp["status"],
		"example status should be present")

	// Resource should be stored and fetchable.
	assert.Equal(t, 1, store.Count(), "store should have 1 resource")
}

func TestHandleCreate_DegradedWithExample_NoRequestBody(t *testing.T) {
	// Degraded path with example but no request body.
	// Should use example values directly (no ID protection).
	store := state.NewInMemory(100)
	h := newStatefulTestHandler(store)
	gen := newStatefulGen()

	exampleData := map[string]any{
		"id":   float64(99),
		"name": "Example Only",
	}

	entry := model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/items",
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

	req := httptest.NewRequest(http.MethodPost, "/items", nil)
	rec := httptest.NewRecorder()

	handled := h.handleStatefulMode(rec, req, entry, gen, []byte{})

	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.InDelta(t, float64(99), resp["id"], 0, "example ID should be present")
	assert.Equal(t, "Example Only", resp["name"], "example name should be present")
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
