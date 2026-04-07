package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Classifier Improvement E2E Tests ---
//
// These tests verify the three classifier improvements end-to-end,
// ensuring the full pipeline (parse → classify → compile → build → route)
// produces correct behavior for previously-misclassified endpoint patterns.
//
// Test spec: testdata/specs/e2e-classifier-test.yaml

// TestE2E_Classifier_PostAsUpdate verifies L3 summary scanning:
// POST to an item path with summary "Update a customer" is classified as
// update (returns 200), not create (returns 201). This pattern is how
// Stripe's API works — POST for both create and update, distinguished by
// whether the path targets a collection or an item.
func TestE2E_Classifier_PostAsUpdate(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-classifier-test.yaml")
	defer srv.Close()

	// POST /customers (collection) → create → 201.
	createResp := doJSONRequest(t, srv, http.MethodPost, "/customers",
		`{"name": "Alice"}`)
	defer closeBody(t, createResp)

	assert.Equal(t, http.StatusCreated, createResp.StatusCode,
		"POST to collection should be classified as create (201)")

	// POST /customers/{id} (item) → update → 200.
	// Before Session 34, this was misclassified as create (201).
	updateResp := doJSONRequest(t, srv, http.MethodPost, "/customers/cus_123",
		`{"name": "Alice Updated"}`)
	defer closeBody(t, updateResp)

	assert.Equal(t, http.StatusOK, updateResp.StatusCode,
		"POST to item path with 'Update' summary should be classified as update (200)")

	// Verify the update response contains schema-valid customer data.
	var customer map[string]any
	decodeJSON(t, updateResp, &customer)

	assert.Contains(t, customer, "id", "update response should have id")
	assert.Contains(t, customer, "name", "update response should have name")
}

// TestE2E_Classifier_SubResourceDelete verifies L1 sub-resource delete detection:
// DELETE /customers/{id}/avatar (singular sub-resource with parent path param)
// is classified as delete (returns 204), not generic.
func TestE2E_Classifier_SubResourceDelete(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-classifier-test.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodDelete, "/customers/cus_123/avatar")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode,
		"DELETE on singular sub-resource should be classified as delete (204)")

	// 204 should have no body.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Empty(t, body, "204 should have no response body")
}

// TestE2E_Classifier_SingletonFetch verifies L3 targeted list→fetch override:
// GET /me with operationId "users-showAuthenticated" (keyword "show" at
// position > 0) is classified as fetch (returns object), not list (array).
func TestE2E_Classifier_SingletonFetch(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-classifier-test.yaml")
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/me")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	// Fetch behavior returns an object; list behavior returns an array.
	// The classifier should pick fetch, so the response should be a User object.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var raw any
	require.NoError(t, json.Unmarshal(body, &raw))

	user, ok := raw.(map[string]any)
	require.True(t, ok,
		"GET /me should return an object (fetch), not an array (list); got %T", raw)

	assert.Contains(t, user, "id", "user should have id")
	assert.Contains(t, user, "username", "user should have username")
}

// TestE2E_Classifier_Determinism verifies that classifier improvements
// don't break the core determinism property across the three new patterns.
func TestE2E_Classifier_Determinism(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-classifier-test.yaml")
	defer srv.Close()

	paths := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/customers/cus_123", `{"name": "Alice"}`},
		{http.MethodGet, "/me", ""},
		{http.MethodGet, "/customers", ""},
		{http.MethodDelete, "/customers/cus_123/avatar", ""},
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			if p.body != "" {
				resp1 := doJSONRequest(t, srv, p.method, p.path, p.body)
				b1, _ := io.ReadAll(resp1.Body)
				closeBody(t, resp1)

				resp2 := doJSONRequest(t, srv, p.method, p.path, p.body)
				b2, _ := io.ReadAll(resp2.Body)
				closeBody(t, resp2)

				assert.Equal(t, string(b1), string(b2),
					"identical requests must produce identical responses")
			} else {
				resp1 := doRequest(t, srv, p.method, p.path)
				b1, _ := io.ReadAll(resp1.Body)
				code1 := resp1.StatusCode
				closeBody(t, resp1)

				resp2 := doRequest(t, srv, p.method, p.path)
				b2, _ := io.ReadAll(resp2.Body)
				code2 := resp2.StatusCode
				closeBody(t, resp2)

				assert.Equal(t, code1, code2,
					"identical requests must produce identical status codes")
				assert.Equal(t, string(b1), string(b2),
					"identical requests must produce identical responses")
			}
		})
	}
}

// TestE2E_Classifier_StrictMode verifies all classifier improvement patterns
// produce schema-valid responses under strict validation.
func TestE2E_Classifier_StrictMode(t *testing.T) {
	skipIfShort(t)

	specBytes := loadSpecBytes(t, "e2e-classifier-test.yaml")

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
		{http.MethodGet, "/customers", "", http.StatusOK},
		{http.MethodPost, "/customers", `{"name": "Alice"}`, http.StatusCreated},
		{http.MethodGet, "/customers/cus_123", "", http.StatusOK},
		{http.MethodPost, "/customers/cus_123", `{"name": "Updated"}`, http.StatusOK},
		{http.MethodDelete, "/customers/cus_123/avatar", "", http.StatusNoContent},
		{http.MethodGet, "/me", "", http.StatusOK},
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
