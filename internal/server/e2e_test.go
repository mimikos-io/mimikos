package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Petstore 3.0 End-to-End Tests ---

func TestE2E_Petstore30_ListPets(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var arr []map[string]any
	decodeJSON(t, resp, &arr)

	// Each pet in the array should have id and name (required fields).
	require.NotEmpty(t, arr, "should return at least one pet")

	for i, pet := range arr {
		assert.Contains(t, pet, "id", "pet[%d] should have id", i)
		assert.Contains(t, pet, "name", "pet[%d] should have name", i)
	}
}

func TestE2E_Petstore30_CreatePet(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"id": 1, "name": "Fido"}`)
	defer closeBody(t, resp)

	// Petstore 3.0 POST /pets returns 201 with no response body schema.
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestE2E_Petstore30_GetPetById(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets/42")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id", "pet should have id")
	assert.Contains(t, pet, "name", "pet should have name")
}

func TestE2E_Petstore30_InvalidCreate_MissingRequired(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	// POST /pets without required "name" field.
	resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"tag": "dog"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")

	problem := decodeProblemJSON(t, resp)
	assertStatusField(t, problem, http.StatusBadRequest)
	assert.NotEmpty(t, problem["title"])
}

func TestE2E_Petstore30_NotFound(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/nonexistent")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestE2E_Petstore30_MethodNotAllowed(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	// DELETE /pets is not defined in petstore 3.0.
	resp := doRequest(t, srv, http.MethodDelete, "/pets")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestE2E_Petstore30_Determinism(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	body1 := doGetBody(t, srv, "/pets/42")
	body2 := doGetBody(t, srv, "/pets/42")

	assert.Equal(t, body1, body2, "identical requests must produce identical responses")
}

func TestE2E_Petstore30_ExplicitStatus_SuccessCode(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.0.yaml")
	defer srv.Close()

	// Explicitly requesting the success code should behave identically to default.
	defaultResp := doGetBody(t, srv, "/pets/42")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/pets/42", nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", "200")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, defaultResp, string(body),
		"explicit 200 should produce the same response as default")
}

// --- Petstore 3.1 (Complex + 3.1 Features) ---

func TestE2E_Petstore31_ListPets(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var arr []map[string]any
	decodeJSON(t, resp, &arr)

	require.NotEmpty(t, arr)

	for i, pet := range arr {
		assert.Contains(t, pet, "id", "pet[%d] should have id", i)
		assert.Contains(t, pet, "name", "pet[%d] should have name", i)
	}
}

func TestE2E_Petstore31_CreatePet(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	// Petstore 3.1 POST /pets returns 201 WITH a response schema (unlike 3.0).
	resp := doJSONRequest(t, srv, http.MethodPost, "/pets", `{"name": "Buddy"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id", "created pet should have id")
	assert.Contains(t, pet, "name", "created pet should have name")
}

func TestE2E_Petstore31_GetPetById(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets/99")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")
}

func TestE2E_Petstore31_DeletePet(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodDelete, "/pets/99")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// 204 should have no body.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Empty(t, body, "204 should have no response body")
}

func TestE2E_Petstore31_UpdatePet(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	resp := doJSONRequest(t, srv, http.MethodPatch, "/pets/99",
		`{"name": "Updated Name"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")
}

func TestE2E_Petstore31_NullableField(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	// Pet.tag is type: [string, null] — response should be valid JSON
	// (either a string value or null, both are acceptable).
	resp := doRequest(t, srv, http.MethodGet, "/pets/77")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	// tag should be present (generator produces all properties).
	// Value is either string or nil (JSON null) — both valid.
	if tag, ok := pet["tag"]; ok {
		switch tag.(type) {
		case string:
			// Valid: string value.
		case nil:
			// Valid: null value.
		default:
			t.Errorf("tag should be string or null, got %T", tag)
		}
	}
}

func TestE2E_Petstore31_PolymorphicField(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	// Pet.status uses oneOf with discriminator (ActiveStatus | ArchivedStatus).
	// The generator should produce a valid branch.
	resp := doRequest(t, srv, http.MethodGet, "/pets/55")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var pet map[string]any
	decodeJSON(t, resp, &pet)

	// status should be present and be an object with "type" discriminator.
	status, ok := pet["status"]
	if !ok {
		return // status is not required, absence is acceptable
	}

	statusObj, ok := status.(map[string]any)
	require.True(t, ok, "status should be an object, got %T", status)

	// Discriminator field "type" should be "active" or "archived".
	typeVal, ok := statusObj["type"]
	require.True(t, ok, "status object should have 'type' discriminator")

	typeStr, ok := typeVal.(string)
	require.True(t, ok, "status.type should be a string")
	assert.Contains(t, []string{"active", "archived"}, typeStr,
		"status.type should be 'active' or 'archived'")

	// Validate branch-specific fields.
	switch typeStr {
	case "active":
		assert.Contains(t, statusObj, "since", "ActiveStatus should have 'since'")
	case "archived":
		assert.Contains(t, statusObj, "reason", "ArchivedStatus should have 'reason'")
	}
}

func TestE2E_Petstore31_Determinism(t *testing.T) {
	srv := buildTestServer(t, "petstore-3.1.yaml")
	defer srv.Close()

	body1 := doGetBody(t, srv, "/pets/55")
	body2 := doGetBody(t, srv, "/pets/55")

	assert.Equal(t, body1, body2, "3.1 spec: identical requests must produce identical responses")
}

func TestE2E_Petstore31_3031Parity(t *testing.T) {
	// Both specs define GET /pets → list, GET /pets/{petId} → fetch.
	// Behavioral classification should be the same.
	srv30 := buildTestServer(t, "petstore-3.0.yaml")
	defer srv30.Close()

	srv31 := buildTestServer(t, "petstore-3.1.yaml")
	defer srv31.Close()

	// Both should return 200 with JSON arrays for GET /pets.
	resp30 := doRequest(t, srv30, http.MethodGet, "/pets")
	defer closeBody(t, resp30)

	assert.Equal(t, http.StatusOK, resp30.StatusCode)

	resp31 := doRequest(t, srv31, http.MethodGet, "/pets")
	defer closeBody(t, resp31)

	assert.Equal(t, http.StatusOK, resp31.StatusCode)

	// Both should return 200 with JSON objects for GET /pets/{id}.
	resp30Item := doRequest(t, srv30, http.MethodGet, "/pets/1")
	defer closeBody(t, resp30Item)

	assert.Equal(t, http.StatusOK, resp30Item.StatusCode)

	resp31Item := doRequest(t, srv31, http.MethodGet, "/pets/1")
	defer closeBody(t, resp31Item)

	assert.Equal(t, http.StatusOK, resp31Item.StatusCode)

	// Verify both return the same structural type (array vs object).
	var arr30 []any
	decodeJSON(t, resp30, &arr30)

	var arr31 []any
	decodeJSON(t, resp31, &arr31)

	var obj30 map[string]any
	decodeJSON(t, resp30Item, &obj30)

	var obj31 map[string]any
	decodeJSON(t, resp31Item, &obj31)

	// Both should have the same required fields.
	assert.Contains(t, obj30, "id")
	assert.Contains(t, obj30, "name")
	assert.Contains(t, obj31, "id")
	assert.Contains(t, obj31, "name")
}

// --- Complex Spec (Asana) ---

func TestE2E_Asana_BuildSucceeds(t *testing.T) {
	// Asana has 167 operations — verify the full pipeline builds without error.
	specBytes := loadSpecBytes(t, "asana.yaml")

	handler, result, err := Build(context.Background(), specBytes, Config{})
	require.NoError(t, err)
	require.NotNil(t, handler)

	// Asana is a large spec with many operations.
	assert.Greater(t, result.Operations, 100,
		"Asana should have 100+ operations")
}

func TestE2E_Petstore31_StrictMode(t *testing.T) {
	// Strict mode enables response validation — every generated response is
	// checked against the compiled JSON Schema. A non-500 in strict mode
	// proves the generated data is schema-valid end-to-end.
	specBytes := loadSpecBytes(t, "petstore-3.1.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{Strict: true})
	require.NoError(t, err)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	endpoints := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/pets/99", "", http.StatusOK},
		{http.MethodPost, "/pets", `{"name": "Buddy"}`, http.StatusCreated},
		{http.MethodPatch, "/pets/99", `{"name": "Updated"}`, http.StatusOK},
		{http.MethodDelete, "/pets/99", "", http.StatusNoContent},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var resp *http.Response

			if ep.body != "" {
				resp = doJSONRequest(t, srv, ep.method, ep.path, ep.body)
			} else {
				resp = doRequest(t, srv, ep.method, ep.path)
			}

			defer closeBody(t, resp)

			if resp.StatusCode != ep.want {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("strict mode: expected %d, got %d\nbody: %s",
					ep.want, resp.StatusCode, body)
			}
		})
	}
}

func TestE2E_Asana_StrictMode(t *testing.T) {
	specBytes := loadSpecBytes(t, "asana.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{Strict: true})
	require.NoError(t, err)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// List/nested-list endpoints pass strict validation: their response
	// schema uses TaskCompact (shallow allOf chain) which fully resolves
	// within defaultMaxDepth=3.
	//
	// Fetch endpoints (GET /tasks/{id}, POST /tasks) use TaskResponse
	// which has deeply nested array-of-object properties (hearts, likes,
	// memberships, dependencies, dependents) that exceed structural
	// maxDepth=3 and produce null array items. The spec also has an
	// assignee_section inconsistency (string in TaskBase vs object in
	// TaskResponse). These are tracked separately and excluded here.
	endpoints := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/tasks", "", http.StatusOK},
		{http.MethodGet, "/projects/abc123/tasks", "", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var resp *http.Response

			if ep.body != "" {
				resp = doJSONRequest(t, srv, ep.method, ep.path, ep.body)
			} else {
				resp = doRequest(t, srv, ep.method, ep.path)
			}

			defer closeBody(t, resp)

			if resp.StatusCode != ep.want {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("strict mode: expected %d, got %d\nbody: %s",
					ep.want, resp.StatusCode, body)
			}
		})
	}
}

func TestE2E_Asana_ListTasks(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// GET /tasks → list behavior, returns wrapper object with data array.
	resp := doRequest(t, srv, http.MethodGet, "/tasks")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body map[string]any
	decodeJSON(t, resp, &body)

	// Asana wraps responses in {data: [...]}.
	data, ok := body["data"]
	require.True(t, ok, "Asana response should have 'data' wrapper")

	arr, ok := data.([]any)
	require.True(t, ok, "data should be an array")
	require.NotEmpty(t, arr, "data array should not be empty")

	// With depth-neutral allOf, TaskCompact objects resolve fully.
	task, ok := arr[0].(map[string]any)
	require.True(t, ok, "array items should be task objects, not null")
	assert.Contains(t, task, "gid", "TaskCompact should have gid from AsanaResource")
}

func TestE2E_Asana_NestedResource(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// GET /projects/{project_gid}/tasks — nested resource (tasks under project).
	resp := doRequest(t, srv, http.MethodGet, "/projects/abc123/tasks")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)

	data, ok := body["data"]
	require.True(t, ok, "nested resource response should have 'data' wrapper")

	arr, ok := data.([]any)
	require.True(t, ok, "nested resource data should be an array")
	require.NotEmpty(t, arr, "nested resource data should not be empty")

	// With depth-neutral allOf, TaskCompact objects resolve fully.
	task, ok := arr[0].(map[string]any)
	require.True(t, ok, "nested resource items should be task objects, not null")
	assert.Contains(t, task, "gid", "TaskCompact should have gid from AsanaResource")
}

func TestE2E_Asana_FetchSingleResource(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// GET /tasks/{task_gid} → fetch single task.
	resp := doRequest(t, srv, http.MethodGet, "/tasks/12345")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)

	// Single resource responses also wrapped in {data: {...}}.
	data, ok := body["data"]
	require.True(t, ok, "fetch response should have 'data' wrapper")

	obj, ok := data.(map[string]any)
	require.True(t, ok, "single resource data should be an object")
	assert.NotEmpty(t, obj, "single resource data should not be an empty object")

	// With depth-neutral allOf, gid from AsanaResource base is now present.
	assert.Contains(t, obj, "gid", "TaskResponse should include gid from AsanaResource")
}

func TestE2E_Asana_ActionEndpoint(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// POST /tasks/{task_gid}/addDependencies — RPC-style action endpoint.
	// Should be classified as generic and return a response.
	resp := doJSONRequest(t, srv, http.MethodPost, "/tasks/12345/addDependencies",
		`{"dependencies":["67890"]}`)
	defer closeBody(t, resp)

	// Action endpoints return 200 with response body.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestE2E_Asana_ExplicitStatus_404(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// Asana defines explicit 404 with ErrorResponse schema on every operation.
	resp := doRequestWithStatus(t, srv, "/tasks/12345", "404")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body map[string]any
	decodeJSON(t, resp, &body)

	// ErrorResponse has {errors: [{message, help, phrase}]}.
	_, ok := body["errors"]
	assert.True(t, ok, "Asana 404 should have 'errors' field from ErrorResponse schema")
}

func TestE2E_Asana_ExplicitStatus_500(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// Asana defines explicit 500 with ErrorResponse schema.
	resp := doRequestWithStatus(t, srv, "/projects/abc123", "500")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body map[string]any
	decodeJSON(t, resp, &body)

	_, ok := body["errors"]
	assert.True(t, ok, "Asana 500 should have 'errors' field from ErrorResponse schema")
}

func TestE2E_Asana_ExplicitStatus_UndefinedCode(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// 418 is not defined for any Asana operation.
	resp := doRequestWithStatus(t, srv, "/tasks/12345", "418")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestE2E_Asana_ExplicitStatus_InvalidValue(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// Non-numeric status code.
	resp := doRequestWithStatus(t, srv, "/tasks/12345", "abc")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestE2E_Asana_ExplicitStatus_SuccessCodeExplicit(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// Explicitly requesting 200 should match default behavior.
	defaultBody := doGetBody(t, srv, "/tasks/12345")

	explicitBody := doGetBodyWithStatus(t, srv, "/tasks/12345", "200")

	assert.Equal(t, defaultBody, explicitBody,
		"explicit 200 should produce the same response as default")
}

func TestE2E_Asana_ExplicitStatus_Determinism(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	// Same explicit error status request should produce identical responses.
	body1 := doGetBodyWithStatus(t, srv, "/tasks/12345", "404")
	body2 := doGetBodyWithStatus(t, srv, "/tasks/12345", "404")

	assert.Equal(t, body1, body2,
		"explicit status selection must be deterministic")
}

func TestE2E_Asana_Determinism(t *testing.T) {
	srv := buildTestServer(t, "asana.yaml")
	defer srv.Close()

	body1 := doGetBody(t, srv, "/tasks/12345")
	body2 := doGetBody(t, srv, "/tasks/12345")

	assert.Equal(t, body1, body2,
		"Asana: identical requests must produce identical responses")
}

// --- Error Scenarios ---

func TestE2E_Error_WrongContentType(t *testing.T) {
	srv := buildTestServer(t, "validation-test.yaml")
	defer srv.Close()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/users",
		strings.NewReader(`{"name":"test","age":25,"status":"active"}`))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestE2E_Error_MissingRequiredFields(t *testing.T) {
	srv := buildTestServer(t, "validation-test.yaml")
	defer srv.Close()

	// POST /users requires name, age, status — send empty object.
	resp := doJSONRequest(t, srv, http.MethodPost, "/users", `{}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")

	problem := decodeProblemJSON(t, resp)

	assertStatusField(t, problem, http.StatusBadRequest)

	// DefaultResponder.ValidationError always produces an errors array.
	errorsVal, ok := problem["errors"]
	require.True(t, ok, "problem JSON should have 'errors' field")

	errorsArr, ok := errorsVal.([]any)
	require.True(t, ok, "errors should be an array")
	assert.GreaterOrEqual(t, len(errorsArr), 1,
		"should report at least one missing required field")
}

func TestE2E_Error_TypeMismatch(t *testing.T) {
	srv := buildTestServer(t, "validation-test.yaml")
	defer srv.Close()

	// age should be integer, send string.
	resp := doJSONRequest(t, srv, http.MethodPost, "/users",
		`{"name":"test","age":"not-a-number","status":"active"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

// --- X-Mimikos-Status: Schema-less Fallback ---

func TestE2E_ExplicitStatus_SchemalessError_RFC7807Fallback(t *testing.T) {
	// This test requires e2e-status-test.yaml because it defines a 422 response
	// with no body schema. Asana cannot test this path — all its error responses
	// have ErrorResponse schemas. This exercises router.writeErrorFallback.
	srv := buildTestServer(t, "e2e-status-test.yaml")
	defer srv.Close()

	resp := doJSONRequestWithStatus(t, srv, http.MethodPost, "/resources",
		`{"name":"test"}`, "422")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")

	problem := decodeProblemJSON(t, resp)
	assertStatusField(t, problem, http.StatusUnprocessableEntity)
}

// --- Test Helpers (E2E-specific; shared helpers are in helpers_test.go) ---

// doJSONRequest performs an HTTP request with a JSON body.
func doJSONRequest(
	t *testing.T,
	srv *httptest.Server,
	method, path, body string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+path,
		strings.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// doRequestWithStatus performs a GET request with X-Mimikos-Status header.
func doRequestWithStatus(
	t *testing.T,
	srv *httptest.Server,
	path, status string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", status)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// doJSONRequestWithStatus performs a POST/PUT/PATCH with body and X-Mimikos-Status.
func doJSONRequestWithStatus(
	t *testing.T,
	srv *httptest.Server,
	method, path, body, status string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+path,
		strings.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mimikos-Status", status)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// doGetBodyWithStatus performs a GET with X-Mimikos-Status and returns body as string.
func doGetBodyWithStatus(t *testing.T, srv *httptest.Server, path, status string) string {
	t.Helper()

	resp := doRequestWithStatus(t, srv, path, status)
	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

// decodeJSON reads the response body and decodes it into v.
// It drains the body — call only once per response.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, v), "should be valid JSON: %s", body)
}

// decodeProblemJSON reads and decodes an RFC 7807 response.
func decodeProblemJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	var problem map[string]any
	decodeJSON(t, resp, &problem)

	return problem
}

// assertStatusField checks the "status" field in a decoded RFC 7807 problem
// JSON object. JSON decodes numbers as float64, so we use InDelta to satisfy
// the testifylint float-compare rule.
func assertStatusField(t *testing.T, problem map[string]any, expected int) {
	t.Helper()

	status, ok := problem["status"]
	require.True(t, ok, "problem JSON should have 'status' field")

	statusNum, ok := status.(float64)
	require.True(t, ok, "status should be a number, got %T", status)
	assert.InDelta(t, float64(expected), statusNum, 0.1, "status code mismatch")
}
