// Stateful E2E tests make many HTTP requests per test function with defer
// closeBody() cleanup. The bodyclose linter loses track of closes through the
// closeBody helper in complex functions — these are false positives.
//
//nolint:bodyclose // all responses are closed via defer closeBody(t, resp)
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/model"
)

// --- Helpers ---

// buildStatefulServer creates an httptest.Server in stateful mode from a spec file.
func buildStatefulServer(t *testing.T, specFile string, maxResources int) *httptest.Server {
	t.Helper()

	specBytes := loadSpecBytes(t, specFile)

	if maxResources == 0 {
		maxResources = 10_000
	}

	handler, _, err := Build(context.Background(), specBytes, Config{
		Mode:         model.ModeStateful,
		MaxResources: maxResources,
	})
	require.NoError(t, err)

	return httptest.NewServer(handler)
}

// --- 15.1 Petstore Lifecycle Tests ---

func TestStatefulE2E_Petstore_CreateAndFetch(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// POST /pets → 201, response has id.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, createResp)

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	require.Contains(t, created, "id", "created pet must have id")
	require.Contains(t, created, "name", "created pet must have name")

	// Extract generated id for subsequent requests.
	petID := coerceID(t, created["id"])

	// GET /pets/{petId} → 200, returns the created pet.
	fetchResp := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	var fetched map[string]any
	decodeJSON(t, fetchResp, &fetched)

	assert.Equal(t, created["id"], fetched["id"], "fetched pet should match created pet id")
	assert.Equal(t, created["name"], fetched["name"], "fetched pet should match created pet name")
}

func TestStatefulE2E_Petstore_ListContainsCreated(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Create a pet.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	// GET /pets → list contains the created pet.
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var items []any
	decodeJSON(t, listResp, &items)

	require.Len(t, items, 1, "list should contain exactly 1 pet after 1 create")

	pet, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, created["id"], pet["id"])
}

func TestStatefulE2E_Petstore_UpdateMerge(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Create a pet.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	petID := coerceID(t, created["id"])

	// PATCH /pets/{petId} → shallow merge: name updated, id preserved.
	updateResp := doJSONRequest(t, srv, http.MethodPatch, "/pets/"+petID, `{"name": "Updated"}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updated map[string]any
	decodeJSON(t, updateResp, &updated)

	assert.Equal(t, "Updated", updated["name"], "name should be updated")
	assert.Equal(t, created["id"], updated["id"], "id should be preserved after merge")

	// GET confirms the update persisted.
	fetchResp := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	var fetched map[string]any
	decodeJSON(t, fetchResp, &fetched)

	assert.Equal(t, "Updated", fetched["name"])
}

func TestStatefulE2E_Petstore_DeleteAndVerify(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Create a pet.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	petID := coerceID(t, created["id"])

	// DELETE /pets/{petId} → 204.
	deleteResp := doRequest(t, srv, http.MethodDelete, "/pets/"+petID)
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// GET after delete → 404.
	fetchResp := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusNotFound, fetchResp.StatusCode)
	assert.Contains(t, fetchResp.Header.Get("Content-Type"), "application/problem+json")

	// List after delete → empty.
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.Empty(t, items, "list should be empty after deleting the only pet")
}

func TestStatefulE2E_Petstore_MultiResource(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Create 3 pets.
	names := []string{"Alpha", "Beta", "Gamma"}
	ids := make([]string, 3)

	for i, name := range names {
		resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "`+name+`"}`)
		defer closeBody(t, resp)

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var pet map[string]any
		decodeJSON(t, resp, &pet)

		ids[i] = coerceID(t, pet["id"])
	}

	// List returns all 3.
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.Len(t, items, 3, "list should contain 3 pets")

	// Delete the middle pet.
	deleteResp := doRequest(t, srv, http.MethodDelete, "/pets/"+ids[1])
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// List returns 2.
	listResp2 := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp2)

	var items2 []any
	decodeJSON(t, listResp2, &items2)

	assert.Len(t, items2, 2, "list should contain 2 pets after deleting 1")
}

func TestStatefulE2E_Petstore_FullLifecycle(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// 1. Create
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Lifecycle"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	petID := coerceID(t, created["id"])

	// 2. Fetch — matches
	fetchResp := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	// 3. Update
	updateResp := doJSONRequest(t, srv, http.MethodPatch, "/pets/"+petID, `{"name": "Changed"}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// 4. Verify update via fetch
	fetchResp2 := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp2)

	var fetched map[string]any
	decodeJSON(t, fetchResp2, &fetched)

	assert.Equal(t, "Changed", fetched["name"])

	// 5. Delete
	deleteResp := doRequest(t, srv, http.MethodDelete, "/pets/"+petID)
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// 6. Verify gone
	fetchResp3 := doRequest(t, srv, http.MethodGet, "/pets/"+petID)
	defer closeBody(t, fetchResp3)

	assert.Equal(t, http.StatusNotFound, fetchResp3.StatusCode)

	// 7. List is empty
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.Empty(t, items)
}

// --- 15.1.3 Error Scenarios in Stateful Mode ---

func TestStatefulE2E_Petstore_FetchNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// GET /pets/999 before any creates → 404.
	resp := doRequest(t, srv, http.MethodGet, "/pets/999")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Petstore_DeleteNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// DELETE /pets/999 before any creates → 404.
	resp := doRequest(t, srv, http.MethodDelete, "/pets/999")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Petstore_XMimikosStatusBypassesState(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Create a pet.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// X-Mimikos-Status bypasses stateful logic entirely (Decision #81).
	// Requesting 200 should return a generated response, NOT the stored resource.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/pets/42", nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", "200")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	// The response should be schema-generated, not the stored "Buddy" pet.
	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")

	// Verify state was NOT mutated — list still has only the originally created pet.
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.Len(t, items, 1, "X-Mimikos-Status must not mutate state")
}

func TestStatefulE2E_Petstore_ValidationStillApplies(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// POST /pets without required "name" → 400 (validation error).
	resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"tag": "dog"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Petstore_MethodNotAllowed(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// PUT /pets is not defined → 405.
	resp := doJSONRequest(t, srv, http.MethodPut, "/pets", `{"name": "test"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- 15.2 Asana Complex Spec Tests ---
//
// Known limitation: Asana wraps responses in {data: {...}} and uses "gid"
// instead of "id". InferResourceIdentity cannot extract the ID from wrapped
// bodies, so the create→fetch-by-id cycle fails (create stores under a
// deterministic UUID, fetch looks up by path param — they never match).
// This is the motivation for Task 16 (research) and Task 17 (refactor).
//
// Additionally, stateful handleList returns a flat JSON array of stored
// responses, not the spec-defined wrapper ({data: [...]}). This means list
// responses in stateful mode violate the Asana response schema. Task 17
// addresses this with schema-aware list wrapping.

func TestStatefulE2E_Asana_CreateAndList(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// POST /projects → 201. Asana wraps request in {data: {...}}.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Test Project"}}`)
	defer closeBody(t, createResp)

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)

	// Asana response is wrapped: {data: {gid: "...", ...}}.
	data, ok := createBody["data"]
	require.True(t, ok, "Asana response should have 'data' wrapper")

	dataObj, ok := data.(map[string]any)
	require.True(t, ok, "data should be an object")
	assert.Contains(t, dataObj, "gid", "project should have gid from AsanaResource")

	// GET /projects → list returns stored resources.
	listResp := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, listResp)

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var items []any
	decodeJSON(t, listResp, &items)

	require.Len(t, items, 1, "list should contain 1 project after 1 create")

	// Stored item is the full wrapped response.
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, item, "data", "stored item preserves the response wrapper")
}

func TestStatefulE2E_Asana_MultipleResourceTypes(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create resources of different types.
	projectResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Project"}}`)
	defer closeBody(t, projectResp)

	require.Equal(t, http.StatusCreated, projectResp.StatusCode)

	goalResp := doJSONRequest(t, srv, http.MethodPost, "/goals",
		`{"data": {"name": "Goal"}}`)
	defer closeBody(t, goalResp)

	require.Equal(t, http.StatusCreated, goalResp.StatusCode)

	tagResp := doJSONRequest(t, srv, http.MethodPost, "/tags",
		`{"data": {"name": "Tag"}}`)
	defer closeBody(t, tagResp)

	require.Equal(t, http.StatusCreated, tagResp.StatusCode)

	// Each resource type lists independently.
	projectList := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, projectList)

	var projects []any
	decodeJSON(t, projectList, &projects)

	assert.Len(t, projects, 1, "projects list should have 1")

	goalList := doRequest(t, srv, http.MethodGet, "/goals")
	defer closeBody(t, goalList)

	var goals []any
	decodeJSON(t, goalList, &goals)

	assert.Len(t, goals, 1, "goals list should have 1")

	tagList := doRequest(t, srv, http.MethodGet, "/tags")
	defer closeBody(t, tagList)

	var tags []any
	decodeJSON(t, tagList, &tags)

	assert.Len(t, tags, 1, "tags list should have 1")
}

func TestStatefulE2E_Asana_DeleteAndVerifyList(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create 2 projects.
	resp1 := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Project A"}}`)
	defer closeBody(t, resp1)

	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	resp2 := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Project B"}}`)
	defer closeBody(t, resp2)

	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	// List has 2.
	listResp := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	require.Len(t, items, 2)

	// Delete uses path param — won't match wrapped responses that used
	// deterministic UUID. But we can verify delete of non-existent returns 404.
	deleteResp := doRequest(t, srv, http.MethodDelete, "/projects/nonexistent")
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusNotFound, deleteResp.StatusCode)

	// List still has 2 — nothing was actually deleted.
	listResp2 := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, listResp2)

	var items2 []any
	decodeJSON(t, listResp2, &items2)

	assert.Len(t, items2, 2, "list unchanged after failed delete")
}

func TestStatefulE2E_Asana_FetchNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// GET /projects/{project_gid} with no prior creates → 404.
	resp := doRequest(t, srv, http.MethodGet, "/projects/abc123")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Asana_XMimikosStatusOverride(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// X-Mimikos-Status bypasses stateful mode entirely.
	resp := doRequestWithStatus(t, srv, "/projects/abc123", "500")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)

	// Asana defines 500 with ErrorResponse schema.
	_, ok := body["errors"]
	assert.True(t, ok, "500 should use Asana's ErrorResponse schema")
}

func TestStatefulE2E_Asana_GenericFallsThrough(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// POST /tasks/{task_gid}/addDependencies is classified as generic.
	// In stateful mode, generic falls through to deterministic.
	resp := doJSONRequest(t, srv, http.MethodPost, "/tasks/12345/addDependencies",
		`{"dependencies":["67890"]}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestStatefulE2E_Asana_CreateMultipleThenList(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create 3 projects with different bodies → distinct stored resources.
	for i := range 3 {
		resp := doJSONRequest(t, srv, http.MethodPost, "/projects",
			`{"data": {"name": "Project `+strconv.Itoa(i)+`"}}`)
		defer closeBody(t, resp)

		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// List returns all 3.
	listResp := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.Len(t, items, 3, "list should contain all 3 created projects")
}

// --- 15.2 Capacity + LRU Eviction Tests ---

func TestStatefulE2E_Petstore_LRUEviction(t *testing.T) {
	skipIfShort(t)

	// Small capacity to force eviction.
	srv := buildStatefulServer(t, "petstore-3.1.yaml", 3)
	defer srv.Close()

	// Create 3 pets — fills capacity.
	ids := make([]string, 3)

	for i := range 3 {
		resp := doJSONRequest(t, srv, http.MethodPost, "/pets",
			`{"name": "Pet`+strconv.Itoa(i)+`"}`)
		defer closeBody(t, resp)

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var pet map[string]any
		decodeJSON(t, resp, &pet)

		ids[i] = coerceID(t, pet["id"])
	}

	// All 3 are fetchable.
	for _, id := range ids {
		resp := doRequest(t, srv, http.MethodGet, "/pets/"+id)
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "pet %s should exist", id)
	}

	// Access pet[0] to make it recently used (LRU promotes on Get).
	touchResp := doRequest(t, srv, http.MethodGet, "/pets/"+ids[0])
	defer closeBody(t, touchResp)

	assert.Equal(t, http.StatusOK, touchResp.StatusCode)

	// Create a 4th pet — triggers eviction of LRU entry.
	resp4 := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Pet3"}`)
	defer closeBody(t, resp4)

	require.Equal(t, http.StatusCreated, resp4.StatusCode)

	var pet4 map[string]any
	decodeJSON(t, resp4, &pet4)

	pet4ID := coerceID(t, pet4["id"])

	// Pet[0] should survive (recently accessed).
	surv := doRequest(t, srv, http.MethodGet, "/pets/"+ids[0])
	defer closeBody(t, surv)

	assert.Equal(t, http.StatusOK, surv.StatusCode, "recently accessed pet should survive eviction")

	// New pet should exist.
	newResp := doRequest(t, srv, http.MethodGet, "/pets/"+pet4ID)
	defer closeBody(t, newResp)

	assert.Equal(t, http.StatusOK, newResp.StatusCode, "newly created pet should exist")

	// Total in list should be 3 (capacity limit).
	listResp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, listResp)

	var items []any
	decodeJSON(t, listResp, &items)

	assert.LessOrEqual(t, len(items), 3, "list should not exceed capacity")
}

func TestStatefulE2E_Petstore_ContentTypeValidation(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// POST with wrong content type → 415.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/pets",
		http.NoBody)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	// Empty body with text/plain — depending on validator, this may be 415 or
	// pass through (no body to validate). The key point: wrong content type
	// with a body should fail.
	req2, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/pets",
		strings.NewReader(`{"name":"test"}`))
	require.NoError(t, err)

	req2.Header.Set("Content-Type", "text/plain")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)

	defer closeBody(t, resp2)

	assert.Equal(t, http.StatusUnsupportedMediaType, resp2.StatusCode)
}

func TestStatefulE2E_Petstore_NotFoundRoute(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	// Completely unknown route → 404 (not a stateful 404).
	resp := doRequest(t, srv, http.MethodGet, "/unknown/path")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

// --- Test Helpers (stateful-specific) ---

// coerceID converts a JSON value (float64 for numbers, string) to a URL-safe string.
func coerceID(t *testing.T, val any) string {
	t.Helper()

	switch v := val.(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		t.Fatalf("unexpected id type %T: %v", val, val)

		return ""
	}
}
