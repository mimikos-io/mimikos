package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
)

// statefulPipelineBehaviorMap creates a BehaviorMap for full-pipeline stateful
// tests. Includes all CRUD operations and a generic action.
func statefulPipelineBehaviorMap() *model.BehaviorMap {
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
			http.StatusCreated: {Name: "Pet", Schema: petObjectSchema()},
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
		Method:      http.MethodPut,
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorUpdate,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Pet", Schema: petObjectSchema()},
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
			http.StatusNotFound: nil,
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	bm.Put(model.BehaviorEntry{
		Method:      http.MethodPost,
		PathPattern: "/pets/{petId}/feed",
		Type:        model.BehaviorGeneric,
		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Result", Schema: petObjectSchema()},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func newStatefulPipelineHandler(store state.Store) *Handler {
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	return NewHandler(
		statefulPipelineBehaviorMap(), nil, &stubValidator{}, resp, gen,
		false, nil, model.ModeStateful, store,
	)
}

// --- Handler-level stateful pipeline tests ---

func TestStatefulPipeline_PostCreateThenGetReturns(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// POST creates a resource.
	postReq := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Fido"}`))
	postReq.Header.Set("Content-Type", "application/json")

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	require.Equal(t, http.StatusCreated, postRec.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(postRec.Body.Bytes(), &created))
	require.Contains(t, created, "id", "created resource should have an id")

	// Extract the generated ID to fetch it.
	id := created["id"]

	var idStr string

	switch v := id.(type) {
	case float64:
		idStr = formatJSONNumber(v)
	case string:
		idStr = v
	default:
		t.Fatalf("unexpected id type: %T", id)
	}

	// GET should return the same resource.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/pets/"+idStr, nil)
	h.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusOK, getRec.Code)

	var fetched map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &fetched))
	assert.Equal(t, created["id"], fetched["id"])
	assert.Equal(t, created["name"], fetched["name"])
}

// formatJSONNumber formats a float64 as a JSON-friendly string (no trailing .0).
func formatJSONNumber(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}

	return strconv.FormatFloat(v, 'f', -1, 64)
}

func TestStatefulPipeline_DeleteThenGetReturns404(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// Pre-populate a resource directly.
	require.NoError(t, store.Put("pets", "", "42", map[string]any{"id": float64(42), "name": "Fido"}))

	// DELETE the resource.
	delRec := httptest.NewRecorder()
	delReq := httptest.NewRequest(http.MethodDelete, "/pets/42", nil)
	h.ServeHTTP(delRec, delReq)

	assert.Equal(t, http.StatusNoContent, delRec.Code)

	// GET should now return 404.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
	h.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusNotFound, getRec.Code)

	pd := parseProblemDetail(t, getRec.Body.Bytes())
	assert.Equal(t, "Not Found", pd["title"])
}

func TestStatefulPipeline_ListChangesAfterPost(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// List should be empty initially.
	listRec1 := httptest.NewRecorder()
	h.ServeHTTP(listRec1, httptest.NewRequest(http.MethodGet, "/pets", nil))

	assert.Equal(t, http.StatusOK, listRec1.Code)

	var items1 []any
	require.NoError(t, json.Unmarshal(listRec1.Body.Bytes(), &items1))
	assert.Empty(t, items1, "list should be empty before any POST")

	// POST creates a resource.
	postReq := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Rex"}`))
	postReq.Header.Set("Content-Type", "application/json")

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	require.Equal(t, http.StatusCreated, postRec.Code)

	// List should now have 1 item.
	listRec2 := httptest.NewRecorder()
	h.ServeHTTP(listRec2, httptest.NewRequest(http.MethodGet, "/pets", nil))

	var items2 []any
	require.NoError(t, json.Unmarshal(listRec2.Body.Bytes(), &items2))
	assert.Len(t, items2, 1, "list should have 1 item after POST")
}

func TestStatefulPipeline_XMimikosStatusOverridesState(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// Pre-populate a resource.
	require.NoError(t, store.Put("pets", "", "42", map[string]any{"id": float64(42), "name": "Fido"}))

	// GET with X-Mimikos-Status: 404 should return deterministic 404,
	// not the stored resource.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
	getReq.Header.Set("X-Mimikos-Status", "404")
	h.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusNotFound, getRec.Code)

	// Resource should still be in the store (no state mutation).
	_, found := store.Get("pets", "", "42")
	assert.True(t, found, "X-Mimikos-Status must not mutate state")
}

func TestStatefulPipeline_XMimikosStatusPostDoesNotCreate(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// POST with X-Mimikos-Status: 201 should generate a deterministic
	// response but NOT store anything.
	postReq := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{"name":"Ghost"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-Mimikos-Status", "201")

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusCreated, postRec.Code)

	// Store should be empty — no resource was created.
	assert.Equal(t, 0, store.Count(), "X-Mimikos-Status POST must not create state")
}

func TestStatefulPipeline_ValidationRunsInStatefulMode(t *testing.T) {
	store := state.NewInMemory(100)
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	// Use a validator that always returns errors.
	v := &stubValidator{
		errs: []model.ValidationError{
			{Field: "/name", Message: "required field missing"},
		},
	}

	h := NewHandler(
		statefulPipelineBehaviorMap(), nil, v, resp, gen,
		false, nil, model.ModeStateful, store,
	)

	postReq := httptest.NewRequest(http.MethodPost, "/pets",
		strings.NewReader(`{}`))
	postReq.Header.Set("Content-Type", "application/json")

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	// Validation should reject before reaching stateful logic.
	assert.Equal(t, http.StatusBadRequest, postRec.Code)
	assert.Equal(t, 0, store.Count(), "validation failure must not create state")
}

func TestStatefulPipeline_GenericFallsThroughToDeterministic(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// Generic action should produce a deterministic response, not interact with state.
	req := httptest.NewRequest(http.MethodPost, "/pets/123/feed", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, store.Count(), "generic action must not create state")

	// Same request should produce same response (deterministic).
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/pets/123/feed", nil))

	assert.Equal(t, rec.Body.String(), rec2.Body.String(),
		"generic action must be deterministic")
}

func TestStatefulPipeline_UpdateMergesThroughPipeline(t *testing.T) {
	store := state.NewInMemory(100)
	h := newStatefulPipelineHandler(store)

	// Pre-populate.
	require.NoError(t, store.Put("pets", "", "42", map[string]any{
		"id": float64(42), "name": "Fido", "owner": "Alice",
	}))

	// PUT with partial body — should merge through the full pipeline.
	putReq := httptest.NewRequest(http.MethodPut, "/pets/42",
		strings.NewReader(`{"name":"Rex"}`))
	putReq.Header.Set("Content-Type", "application/json")

	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	assert.Equal(t, http.StatusOK, putRec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &body))

	// Merged: name updated, owner preserved.
	assert.Equal(t, "Rex", body["name"])
	assert.Equal(t, "Alice", body["owner"])
	assert.InDelta(t, float64(42), body["id"], 0)
}

// --- Benchmark ---

func BenchmarkStatefulPipeline_GetFetch(b *testing.B) {
	store := state.NewInMemory(100)
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0, nil)

	h := NewHandler(
		statefulPipelineBehaviorMap(), nil, &stubValidator{}, resp, gen,
		false, nil, model.ModeStateful, store,
	)

	// Pre-populate.
	_ = store.Put("pets", "", "42", map[string]any{"id": float64(42), "name": "Fido"})

	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/pets/42", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}
