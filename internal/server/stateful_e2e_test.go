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

// --- Petstore Lifecycle Tests ---

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

// --- Petstore Error Scenarios ---

func TestStatefulE2E_Petstore_FetchNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets/999")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Petstore_DeleteNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

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
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/pets/42", nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", "200")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")

	// Verify state was NOT mutated.
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

	resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"tag": "dog"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Petstore_MethodNotAllowed(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

	resp := doJSONRequest(t, srv, http.MethodPut, "/pets", `{"name": "test"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Asana Complex Spec Tests ---
//
// Asana wraps responses in {data: {...}} and uses "gid" instead of "id".
// Task 17 adds wrapper detection, unwrap-on-store/rewrap-on-read, and
// IDFieldHint to make the full CRUD lifecycle work.

func TestStatefulE2E_Asana_CreateAndFetch(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// POST /projects → 201. Asana wraps response in {data: {...}}.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Test Project"}}`)
	defer closeBody(t, createResp)

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)

	// Response is wrapped: {data: {gid: "...", ...}}.
	data, ok := createBody["data"]
	require.True(t, ok, "Asana response should have 'data' wrapper")

	dataObj, ok := data.(map[string]any)
	require.True(t, ok, "data should be an object")
	require.Contains(t, dataObj, "gid", "project should have gid")

	gid := coerceID(t, dataObj["gid"])

	// GET /projects/{project_gid} → 200, returns the created project wrapped.
	fetchResp := doRequest(t, srv, http.MethodGet, "/projects/"+gid)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	var fetchBody map[string]any
	decodeJSON(t, fetchResp, &fetchBody)

	// Fetch response is re-wrapped: {data: {gid: "...", ...}}.
	fetchData, ok := fetchBody["data"]
	require.True(t, ok, "fetch response should have 'data' wrapper")

	fetchObj, ok := fetchData.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, dataObj["gid"], fetchObj["gid"], "fetched gid should match created gid")
}

func TestStatefulE2E_Asana_ListWrapped(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create a project.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Project"}}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// GET /projects → list returns {data: [...]} not bare array.
	listResp := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, listResp)

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)

	// Verify list is wrapped in {data: [...]}.
	dataField, ok := listBody["data"]
	require.True(t, ok, "list response should have 'data' wrapper key")

	items, ok := dataField.([]any)
	require.True(t, ok, "data field should be an array")
	require.Len(t, items, 1, "list should contain 1 project")

	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, item, "gid", "list items should be unwrapped resources (not double-wrapped)")
}

func TestStatefulE2E_Asana_FullLifecycle(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// 1. Create
	createResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Lifecycle Project"}}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)

	data, ok := createBody["data"].(map[string]any)
	require.True(t, ok, "data should be an object")

	gid := coerceID(t, data["gid"])

	// 2. Fetch
	fetchResp := doRequest(t, srv, http.MethodGet, "/projects/"+gid)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	// 3. Update — PATCH with wrapped body.
	updateResp := doJSONRequest(t, srv, http.MethodPut, "/projects/"+gid,
		`{"data": {"name": "Updated"}}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updateBody map[string]any
	decodeJSON(t, updateResp, &updateBody)

	updatedData, ok := updateBody["data"].(map[string]any)
	require.True(t, ok, "update data should be an object")
	assert.Equal(t, "Updated", updatedData["name"], "name should be updated")
	assert.Equal(t, data["gid"], updatedData["gid"], "gid should be preserved")

	// 4. Verify via fetch
	fetchResp2 := doRequest(t, srv, http.MethodGet, "/projects/"+gid)
	defer closeBody(t, fetchResp2)

	var fetch2Body map[string]any
	decodeJSON(t, fetchResp2, &fetch2Body)

	fetch2Data, ok := fetch2Body["data"].(map[string]any)
	require.True(t, ok, "fetch data should be an object")
	assert.Equal(t, "Updated", fetch2Data["name"])

	// 5. Delete — Asana returns 200 with {data: {}}, not 204.
	deleteResp := doRequest(t, srv, http.MethodDelete, "/projects/"+gid)
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusOK, deleteResp.StatusCode)

	var deleteBody map[string]any
	decodeJSON(t, deleteResp, &deleteBody)

	// Asana delete responses are wrapped in {data: ...}.
	_, hasData := deleteBody["data"]
	assert.True(t, hasData, "Asana delete response should have 'data' wrapper")

	// 6. Verify gone
	fetchResp3 := doRequest(t, srv, http.MethodGet, "/projects/"+gid)
	defer closeBody(t, fetchResp3)

	assert.Equal(t, http.StatusNotFound, fetchResp3.StatusCode)
}

func TestStatefulE2E_Asana_UpdateUnwrap(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create a project.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/projects",
		`{"data": {"name": "Original"}}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)

	data, ok := createBody["data"].(map[string]any)
	require.True(t, ok, "create data should be an object")

	gid := coerceID(t, data["gid"])

	// PATCH with wrapped request body {data: {name: "Updated"}}.
	// parseRequestBody should unwrap before shallowMerge — otherwise
	// the stored resource gets corrupted with a "data" key.
	updateResp := doJSONRequest(t, srv, http.MethodPut, "/projects/"+gid,
		`{"data": {"name": "Updated"}}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Fetch and verify the resource is not corrupted.
	fetchResp := doRequest(t, srv, http.MethodGet, "/projects/"+gid)
	defer closeBody(t, fetchResp)

	var fetchBody map[string]any
	decodeJSON(t, fetchResp, &fetchBody)

	fetchData, ok := fetchBody["data"].(map[string]any)
	require.True(t, ok, "fetch response should be wrapped")

	// The inner resource should have "name" updated, NOT a nested "data" key.
	assert.Equal(t, "Updated", fetchData["name"])
	_, hasNestedData := fetchData["data"]
	assert.False(t, hasNestedData, "resource should not have corrupted 'data' key from merge")
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

	// Each resource type lists independently with wrapped response.
	projectList := doRequest(t, srv, http.MethodGet, "/projects")
	defer closeBody(t, projectList)

	var projectBody map[string]any
	decodeJSON(t, projectList, &projectBody)

	projectItems, ok := projectBody["data"].([]any)
	require.True(t, ok, "projects list data should be an array")
	assert.Len(t, projectItems, 1, "projects list should have 1")
}

func TestStatefulE2E_Asana_FetchNonExistent(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/projects/abc123")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestStatefulE2E_Asana_XMimikosStatusOverride(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	resp := doRequestWithStatus(t, srv, "/projects/abc123", "500")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)

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

// --- 17.4 Nested Resource E2E Test ---
//
// Known limitation: /projects/{project_gid}/tasks and /tasks share the "tasks"
// store namespace because ResourceType() extracts the last non-param segment.
// Composite store keys (e.g., "projects/{gid}/tasks" vs "tasks") would fix this
// but require changes to the store key model — deferred.

func TestStatefulE2E_Asana_NestedResourceNamespace(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "asana.yaml", 0)
	defer srv.Close()

	// Create a task via POST /tasks (the only task create endpoint in Asana spec).
	createResp := doJSONRequest(t, srv, http.MethodPost, "/tasks",
		`{"data": {"name": "Root Task"}}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// GET /tasks lists the task.
	rootList := doRequest(t, srv, http.MethodGet, "/tasks")
	defer closeBody(t, rootList)

	var rootBody map[string]any
	decodeJSON(t, rootList, &rootBody)

	rootItems, ok := rootBody["data"].([]any)
	require.True(t, ok, "tasks list data should be an array")
	require.Len(t, rootItems, 1, "/tasks should have 1 task")

	// GET /projects/{project_gid}/tasks shares the "tasks" namespace.
	// The nested path sees the same tasks as the root path.
	// This documents the namespace collision: ResourceType() extracts "tasks"
	// from both paths, so they share the same store namespace.
	nestedList := doRequest(t, srv, http.MethodGet, "/projects/proj1/tasks")
	defer closeBody(t, nestedList)

	var nestedBody map[string]any
	decodeJSON(t, nestedList, &nestedBody)

	nestedItems, ok := nestedBody["data"].([]any)
	require.True(t, ok, "nested tasks list data should be an array")
	assert.Len(t, nestedItems, 1, "nested /projects/{gid}/tasks shares 'tasks' namespace (known limitation)")
}

// --- Targeted Pattern Tests (e2e-stateful-test.yaml) ---
//
// These test patterns not covered by Petstore (flat + bare array + "id") or
// Asana (wrapped everything + "gid"). The spec is purpose-built.

func TestStatefulE2E_FlatResource_WrappedList(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "e2e-stateful-test.yaml", 0)
	defer srv.Close()

	// Stripe pattern: POST /customers → flat {id, name, email}.
	// Path param is {customer} which doesn't suffix-strip to a body field.
	// ID extraction uses Strategy 4 (body "id" fallback).
	createResp := doJSONRequest(t, srv, http.MethodPost, "/customers", `{"name": "Alice"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	// Response is flat (no wrapper).
	require.Contains(t, created, "id")
	require.Contains(t, created, "name")

	customerID := coerceID(t, created["id"])

	// GET /customers/{customer} → flat resource.
	fetchResp := doRequest(t, srv, http.MethodGet, "/customers/"+customerID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusOK, fetchResp.StatusCode)

	var fetched map[string]any
	decodeJSON(t, fetchResp, &fetched)

	assert.Equal(t, created["id"], fetched["id"])

	// GET /customers → object-wrapped list with "results" key (not "data").
	listResp := doRequest(t, srv, http.MethodGet, "/customers")
	defer closeBody(t, listResp)

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)

	// List is wrapped in {results: [...], has_more: bool, total: int}.
	results, ok := listBody["results"].([]any)
	require.True(t, ok, "list should have 'results' array key")
	require.Len(t, results, 1)

	// Pagination metadata should be present (generated from schema).
	assert.Contains(t, listBody, "has_more", "pagination metadata should be generated")
}

func TestStatefulE2E_NonDataArrayKey(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "e2e-stateful-test.yaml", 0)
	defer srv.Close()

	// Create a product — flat response.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/products", `{"title": "Widget"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// GET /products → list wrapped with "items" key.
	listResp := doRequest(t, srv, http.MethodGet, "/products")
	defer closeBody(t, listResp)

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)

	items, ok := listBody["items"].([]any)
	require.True(t, ok, "list should have 'items' array key")
	require.Len(t, items, 1)

	// Pagination metadata with "page" key should be generated.
	assert.Contains(t, listBody, "page", "pagination metadata should be generated")
}

func TestStatefulE2E_IDFallback_StripePattern(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "e2e-stateful-test.yaml", 0)
	defer srv.Close()

	// Path param is {customer} — not a standard ID param name.
	// singularize("customers") = "customer", stripResourcePrefix("customer", "customers")
	// → no remainder (param == singular). Falls through to Strategy 4: body["id"].
	createResp := doJSONRequest(t, srv, http.MethodPost, "/customers", `{"name": "Bob"}`)
	defer closeBody(t, createResp)

	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	customerID := coerceID(t, created["id"])

	// Full CRUD lifecycle works via body["id"] fallback.
	// Update
	updateResp := doJSONRequest(t, srv, http.MethodPut, "/customers/"+customerID,
		`{"name": "Bob Updated"}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updated map[string]any
	decodeJSON(t, updateResp, &updated)

	assert.Equal(t, "Bob Updated", updated["name"])

	// Delete
	deleteResp := doRequest(t, srv, http.MethodDelete, "/customers/"+customerID)
	defer closeBody(t, deleteResp)

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Verify gone
	fetchResp := doRequest(t, srv, http.MethodGet, "/customers/"+customerID)
	defer closeBody(t, fetchResp)

	assert.Equal(t, http.StatusNotFound, fetchResp.StatusCode)
}

func TestStatefulE2E_PaginationMetadataDeterminism(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "e2e-stateful-test.yaml", 0)
	defer srv.Close()

	// Create 1 customer.
	create1 := doJSONRequest(t, srv, http.MethodPost, "/customers", `{"name": "A"}`)
	defer closeBody(t, create1)

	require.Equal(t, http.StatusCreated, create1.StatusCode)

	// GET /customers → 1 item, captures pagination metadata.
	list1 := doRequest(t, srv, http.MethodGet, "/customers")
	defer closeBody(t, list1)

	var body1 map[string]any
	decodeJSON(t, list1, &body1)

	hasMore1 := body1["has_more"]

	// Create a 2nd customer.
	create2 := doJSONRequest(t, srv, http.MethodPost, "/customers", `{"name": "B"}`)
	defer closeBody(t, create2)

	require.Equal(t, http.StatusCreated, create2.StatusCode)

	// GET /customers → 2 items, same pagination metadata.
	list2 := doRequest(t, srv, http.MethodGet, "/customers")
	defer closeBody(t, list2)

	var body2 map[string]any
	decodeJSON(t, list2, &body2)

	hasMore2 := body2["has_more"]

	// Pagination metadata is generated from schema seed (deterministic),
	// not derived from actual store state. Same seed → same metadata.
	// This is a known limitation documented in the design (§4.3).
	assert.Equal(t, hasMore1, hasMore2,
		"pagination metadata should be deterministic regardless of item count (known limitation)")
}

// --- Capacity + LRU Eviction Tests ---

func TestStatefulE2E_Petstore_LRUEviction(t *testing.T) {
	skipIfShort(t)

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

	// POST with wrong content type + body → 415.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/pets",
		strings.NewReader(`{"name":"test"}`))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
}

func TestStatefulE2E_Petstore_NotFoundRoute(t *testing.T) {
	skipIfShort(t)

	srv := buildStatefulServer(t, "petstore-3.1.yaml", 0)
	defer srv.Close()

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
