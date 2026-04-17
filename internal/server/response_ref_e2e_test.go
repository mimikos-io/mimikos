package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Response-Level $ref End-to-End Tests ---
//
// These tests verify that endpoints using response-level $ref (shared error
// responses) are NOT degraded. Before the fix, the parser built wrong pointers
// for these responses, causing the compiler to fail and degrade the endpoint.

func TestE2E_ResponseRef_SharedErrorNotDegraded(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-response-ref-test.yaml")
	defer srv.Close()

	// GET /items — 200 is inline array, 403 is response-level $ref.
	// If the $ref pointer is wrong, requesting 403 via X-Mimikos-Status returns 500.
	t.Run("GET_items_403_via_header", func(t *testing.T) {
		resp := doRequestWithStatus(t, srv, "/items", "403")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"403 response from $ref should not be degraded")
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

		var body map[string]any
		decodeJSON(t, resp, &body)
		assert.Contains(t, body, "error", "403 response should have 'error' field from Forbidden schema")
	})

	t.Run("POST_items_429_via_header", func(t *testing.T) {
		resp := doJSONRequestWithStatus(t, srv, http.MethodPost, "/items",
			`{"name": "Widget"}`, "429")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
			"429 response from $ref should not be degraded")
	})

	t.Run("GET_item_404_via_header", func(t *testing.T) {
		resp := doRequestWithStatus(t, srv, "/items/42", "404")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"404 response from $ref should not be degraded")

		var body map[string]any
		decodeJSON(t, resp, &body)
		assert.Contains(t, body, "code", "404 response should have ErrorObject fields")
		assert.Contains(t, body, "message")
	})

	t.Run("DELETE_item_403_via_header", func(t *testing.T) {
		// DELETE /items/{id} has 403 as a response-level $ref.
		resp := doDeleteWithStatus(t, srv, "/items/42", "403")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"DELETE 403 response from $ref should not be degraded")
	})
}

func TestE2E_ResponseRef_SuccessResponsesWork(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-response-ref-test.yaml")
	defer srv.Close()

	// Verify that the fix doesn't break normal inline/schema-ref responses.
	t.Run("GET_items_200", func(t *testing.T) {
		resp := doRequest(t, srv, http.MethodGet, "/items")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var arr []map[string]any
		decodeJSON(t, resp, &arr)
		require.NotEmpty(t, arr)

		for i, item := range arr {
			assert.Contains(t, item, "id", "item[%d] should have id", i)
			assert.Contains(t, item, "name", "item[%d] should have name", i)
		}
	})

	t.Run("GET_item_200", func(t *testing.T) {
		resp := doRequest(t, srv, http.MethodGet, "/items/42")
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]any
		decodeJSON(t, resp, &body)
		assert.Contains(t, body, "id")
		assert.Contains(t, body, "name")
	})
}

func TestE2E_ResponseRef_RequestBodyRef(t *testing.T) {
	skipIfShort(t)

	srv := buildTestServer(t, "e2e-response-ref-test.yaml")
	defer srv.Close()

	// POST /items uses a $ref request body. Verify request validation works
	// (requires the compiler to find the schema at the component pointer).
	t.Run("valid_request_accepted", func(t *testing.T) {
		resp := doJSONRequest(t, srv, http.MethodPost, "/items", `{"name": "Widget"}`)
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusCreated, resp.StatusCode,
			"valid request body should be accepted")
	})

	t.Run("invalid_request_rejected", func(t *testing.T) {
		// Missing required field "name".
		resp := doJSONRequest(t, srv, http.MethodPost, "/items", `{}`)
		defer closeBody(t, resp)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"request body missing required field should be rejected")
	})
}

// doDeleteWithStatus performs a DELETE request with X-Mimikos-Status header.
func doDeleteWithStatus(
	t *testing.T,
	srv *httptest.Server,
	path, status string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodDelete, srv.URL+path, nil)
	require.NoError(t, err)

	req.Header.Set("X-Mimikos-Status", status)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}
