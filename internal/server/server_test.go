package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the absolute path to the testdata directory root.
func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

// loadSpecBytes reads a spec file from testdata/specs/.
func loadSpecBytes(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(testdataDir(t), "specs", name))
	require.NoError(t, err)

	return data
}

func TestBuild_Petstore(t *testing.T) {
	specBytes := loadSpecBytes(t, "petstore-3.0.yaml")

	handler, result, err := Build(context.Background(), specBytes, Config{})
	require.NoError(t, err)
	require.NotNil(t, handler)
	require.NotNil(t, result)

	assert.Equal(t, "Swagger Petstore", result.SpecTitle)
	assert.Equal(t, "3.0.0", result.SpecVersion)
	assert.Equal(t, 3, result.Operations)
	assert.Len(t, result.Entries, 3)

	// Verify entries contain expected operations.
	entryMap := make(map[string]EntryInfo)
	for _, e := range result.Entries {
		entryMap[e.Method+" "+e.PathPattern] = e
	}

	listEntry, ok := entryMap["GET /pets"]
	require.True(t, ok, "should have GET /pets")
	assert.Equal(t, "list", listEntry.BehaviorType)

	createEntry, ok := entryMap["POST /pets"]
	require.True(t, ok, "should have POST /pets")
	assert.Equal(t, "create", createEntry.BehaviorType)

	fetchEntry, ok := entryMap["GET /pets/{petId}"]
	require.True(t, ok, "should have GET /pets/{petId}")
	assert.Equal(t, "fetch", fetchEntry.BehaviorType)
}

func TestBuild_InvalidSpec(t *testing.T) {
	_, _, err := Build(context.Background(), []byte("not valid yaml ["), Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid OpenAPI spec")
}

func TestBuild_EmptySpec(t *testing.T) {
	_, _, err := Build(context.Background(), nil, Config{})
	require.Error(t, err)
}

func TestBuild_CancelledContext(t *testing.T) {
	specBytes := loadSpecBytes(t, "petstore-3.0.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := Build(ctx, specBytes, Config{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestBuild_Petstore31(t *testing.T) {
	specBytes := loadSpecBytes(t, "petstore-3.1.yaml")

	handler, result, err := Build(context.Background(), specBytes, Config{})
	require.NoError(t, err)
	require.NotNil(t, handler)
	assert.Equal(t, "3.1.0", result.SpecVersion)
	assert.Equal(t, 5, result.Operations)
}

func TestBuild_StrictMode(t *testing.T) {
	specBytes := loadSpecBytes(t, "petstore-3.0.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{Strict: true})
	require.NoError(t, err)
	require.NotNil(t, handler)
}

func TestBuild_CustomMaxDepth(t *testing.T) {
	specBytes := loadSpecBytes(t, "petstore-3.0.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{MaxDepth: 5})
	require.NoError(t, err)
	require.NotNil(t, handler)
}

// --- Integration Tests ---
// These tests exercise the full pipeline: spec → Build → HTTP → response.

func TestIntegration_GetPets(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets")

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	// Body should be a valid JSON array.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var arr []any
	require.NoError(t, json.Unmarshal(body, &arr), "response should be a JSON array")
}

func TestIntegration_CreatePet(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/pets",
		strings.NewReader(`{"id": 1, "name": "Fido"}`))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestIntegration_GetPetById(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/pets/42")

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(body, &obj), "response should be a JSON object")
}

func TestIntegration_NotFound(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/nonexistent")

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

func TestIntegration_MethodNotAllowed(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodDelete, "/pets")

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestIntegration_Determinism(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Same request twice should produce identical responses.
	body1 := doGetBody(t, srv, "/pets/42")
	body2 := doGetBody(t, srv, "/pets/42")

	assert.Equal(t, body1, body2, "identical requests should produce identical responses")
}

func TestIntegration_ExplicitStatusCode_Unavailable(t *testing.T) {
	handler := buildPetstoreHandler(t)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Petstore 3.0 defines 200 + default for GET /pets — no explicit 500.
	// Requesting an undefined status code returns InvalidScenario (400).
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/pets", nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", "500")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")
}

// buildPetstoreHandler creates a handler from the petstore-3.0.yaml spec.
func buildPetstoreHandler(t *testing.T) http.Handler {
	t.Helper()

	specBytes := loadSpecBytes(t, "petstore-3.0.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{})
	require.NoError(t, err)

	return handler
}

// doRequest performs an HTTP request to the test server and returns the response.
func doRequest(t *testing.T, srv *httptest.Server, method, path string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+path, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// doGetBody performs a GET request and returns the response body as a string.
func doGetBody(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()

	resp := doRequest(t, srv, http.MethodGet, path)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}
