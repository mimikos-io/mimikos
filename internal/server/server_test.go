package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Build() Unit Tests ---

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

// --- Shared Test Helpers ---

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

// buildTestServer creates an httptest.Server from a spec file using default config.
func buildTestServer(t *testing.T, specFile string) *httptest.Server {
	t.Helper()

	specBytes := loadSpecBytes(t, specFile)

	handler, _, err := Build(context.Background(), specBytes, Config{})
	require.NoError(t, err)

	return httptest.NewServer(handler)
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
	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

// closeBody closes the response body, failing the test on error.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}
