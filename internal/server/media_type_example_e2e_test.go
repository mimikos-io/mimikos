package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Media-Type Example E2E Tests (Task 25) ---

// buildMediaTypeExampleTestServer creates an httptest.Server from the
// media-type example test spec with strict mode enabled.
func buildMediaTypeExampleTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	specBytes := loadSpecBytes(t, "e2e-media-type-example-test.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{Strict: true})
	require.NoError(t, err)

	return httptest.NewServer(handler)
}

// --- Level 1: Media-type examples returned as-is ---

func TestMediaTypeExampleE2E_SingularExample_ReturnedAsIs(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/items")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var items []map[string]any
	decodeJSON(t, resp, &items)

	require.Len(t, items, 2, "singular example has 2-item array")
	assert.Equal(t, "Widget", items[0]["name"])
	assert.InDelta(t, 9.99, items[0]["price"], 0.001)
	assert.Equal(t, "Gadget", items[1]["name"])
	assert.InDelta(t, 19.99, items[1]["price"], 0.001)
}

func TestMediaTypeExampleE2E_PluralExamples_FirstEntryReturned(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	item := getJSONObject(t, srv, "/items/1")

	// Decision #110: first named example wins (spec order).
	assert.InDelta(t, 42.0, item["id"], 0.1, "first named example 'widget' should be used")
	assert.Equal(t, "Widget", item["name"])
	assert.InDelta(t, 9.99, item["price"], 0.001)
}

func TestMediaTypeExampleE2E_MediaTypeExampleWinsOverSchema(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	mixed := getJSONObject(t, srv, "/mixed")

	// Media-type example should be returned, not generated from schema.
	assert.Equal(t, "authored", mixed["alpha"])
	assert.InDelta(t, 42.0, mixed["beta"], 0.1)
}

func TestMediaTypeExampleE2E_XMimikosStatus_SelectsPerStatusExample(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	// Request the 404 status for GET /items/{itemId}.
	resp := doRequestWithStatus(t, srv, "/items/1", "404")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errBody map[string]any
	decodeJSON(t, resp, &errBody)

	assert.InDelta(t, 404.0, errBody["code"], 0.1)
	assert.Equal(t, "Item not found", errBody["message"])
}

func TestMediaTypeExampleE2E_PostExample_201(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	resp := doJSONRequest(t, srv, http.MethodPost, "/items", `{"name":"Test"}`)
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var item map[string]any
	decodeJSON(t, resp, &item)

	assert.InDelta(t, 100.0, item["id"], 0.1)
	assert.Equal(t, "New Thing", item["name"])
	assert.InDelta(t, 5.5, item["price"], 0.001)
}

// --- CF2: $ref in named examples ---

func TestMediaTypeExampleE2E_RefExample_Resolved(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/items/1/reviews")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var reviews []map[string]any
	decodeJSON(t, resp, &reviews)

	require.Len(t, reviews, 2, "$ref example should resolve to 2-item array")
	assert.InDelta(t, 5.0, reviews[0]["rating"], 0.1)
	assert.Equal(t, "Excellent", reviews[0]["comment"])
	assert.InDelta(t, 3.0, reviews[1]["rating"], 0.1)
	assert.Equal(t, "Average", reviews[1]["comment"])
}

// --- Level 2: Schema-level examples for objects/arrays ---

func TestMediaTypeExampleE2E_Level2_ObjectSchemaExample(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	obj := getJSONObject(t, srv, "/nested")

	// "metadata" has a schema-level example: {author: "Alice", version: 3}.
	// Level 2 should return the authored example for the sub-object.
	meta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be an object, got %T: %v", obj["metadata"], obj["metadata"])
	assert.Equal(t, "Alice", meta["author"])
	assert.InDelta(t, 3.0, meta["version"], 0.1)
}

func TestMediaTypeExampleE2E_Level2_ArraySchemaExample(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	obj := getJSONObject(t, srv, "/tags")

	// "tags" has a schema-level example: ["go", "openapi", "mock"].
	// Level 2 should return the authored example for the array.
	tags, ok := obj["tags"].([]any)
	require.True(t, ok, "tags should be an array, got %T: %v", obj["tags"], obj["tags"])
	assert.Equal(t, []any{"go", "openapi", "mock"}, tags)
}

// --- Cross-cutting concerns ---

func TestMediaTypeExampleE2E_SchemaOnlyFallback(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	obj := getJSONObject(t, srv, "/schema-only")

	// No media-type example — should generate from schema.
	assert.Contains(t, obj, "name", "generated response should have name field")
	assert.Contains(t, obj, "count", "generated response should have count field")
}

func TestMediaTypeExampleE2E_Determinism(t *testing.T) {
	skipIfShort(t)

	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	// Level 1: media-type examples are authored data — trivially deterministic.
	body1 := doGetBody(t, srv, "/items/1")
	body2 := doGetBody(t, srv, "/items/1")
	assert.Equal(t, body1, body2, "Level 1: same request should produce identical response")

	// Schema-only: generation must also be deterministic.
	body3 := doGetBody(t, srv, "/schema-only")
	body4 := doGetBody(t, srv, "/schema-only")
	assert.Equal(t, body3, body4, "schema-only: same request should produce identical response")
}

func TestMediaTypeExampleE2E_StrictMode_ExamplePassesValidation(t *testing.T) {
	skipIfShort(t)

	// Server is built with Strict: true. Media-type examples bypass
	// validation (Decision #108), so they must not trigger 500.
	srv := buildMediaTypeExampleTestServer(t)
	defer srv.Close()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"Level 1 singular", http.MethodGet, "/items", "", http.StatusOK},
		{"Level 1 plural", http.MethodGet, "/items/1", "", http.StatusOK},
		{"Level 1 mixed", http.MethodGet, "/mixed", "", http.StatusOK},
		{"Level 1 POST", http.MethodPost, "/items", `{"name":"Test"}`, http.StatusCreated},
		{"Level 2 object", http.MethodGet, "/nested", "", http.StatusOK},
		{"Level 2 array", http.MethodGet, "/tags", "", http.StatusOK},
		{"Schema-only", http.MethodGet, "/schema-only", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.body != "" {
				resp = doJSONRequest(t, srv, tt.method, tt.path, tt.body)
			} else {
				resp = doRequest(t, srv, tt.method, tt.path)
			}
			defer closeBody(t, resp)

			assert.Equal(t, tt.status, resp.StatusCode,
				"strict mode should not cause 500 for %s", tt.name)
		})
	}
}
