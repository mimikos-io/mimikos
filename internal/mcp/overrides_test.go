package mcp

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- StatusOverrides tests ---

func TestStatusOverrides_SetAndConsume(t *testing.T) {
	so := NewStatusOverrides()

	so.Set("GET", "/pets", 404)

	code, ok := so.Consume("GET", "/pets")

	assert.True(t, ok)
	assert.Equal(t, 404, code)
}

func TestStatusOverrides_ConsumeRemovesOverride(t *testing.T) {
	so := NewStatusOverrides()

	so.Set("GET", "/pets", 500)

	code, ok := so.Consume("GET", "/pets")
	assert.True(t, ok)
	assert.Equal(t, 500, code)

	// Second consume should return false — one-shot semantics.
	_, ok = so.Consume("GET", "/pets")
	assert.False(t, ok, "override should be consumed after first use")
}

func TestStatusOverrides_ConsumeMissingKey(t *testing.T) {
	so := NewStatusOverrides()

	_, ok := so.Consume("GET", "/nonexistent")
	assert.False(t, ok)
}

func TestStatusOverrides_MultipleEndpoints(t *testing.T) {
	so := NewStatusOverrides()

	so.Set("GET", "/pets", 404)
	so.Set("POST", "/pets", 422)
	so.Set("DELETE", "/pets/{petId}", 403)

	code, ok := so.Consume("POST", "/pets")
	assert.True(t, ok)
	assert.Equal(t, 422, code)

	// Others should still be present.
	code, ok = so.Consume("GET", "/pets")
	assert.True(t, ok)
	assert.Equal(t, 404, code)

	code, ok = so.Consume("DELETE", "/pets/{petId}")
	assert.True(t, ok)
	assert.Equal(t, 403, code)
}

func TestStatusOverrides_SetOverwritesPrevious(t *testing.T) {
	so := NewStatusOverrides()

	so.Set("GET", "/pets", 404)
	so.Set("GET", "/pets", 503)

	code, ok := so.Consume("GET", "/pets")
	assert.True(t, ok)
	assert.Equal(t, 503, code, "latest Set should overwrite")
}

func TestStatusOverrides_ConcurrentSetAndConsume(_ *testing.T) {
	so := NewStatusOverrides()

	var wg sync.WaitGroup

	// Concurrent setters.
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			so.Set("GET", "/concurrent", 400+n)
		}(i)
	}

	// Concurrent consumers.
	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			so.Consume("GET", "/concurrent")
		}()
	}

	wg.Wait()
	// No race detector failures is the success criterion.
}

// --- StatusOverride middleware tests ---

func TestStatusOverrideMiddleware_InjectsHeader(t *testing.T) {
	so := NewStatusOverrides()
	so.Set("GET", "/pets", 404)

	var receivedHeader string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Mimikos-Status")

		w.WriteHeader(http.StatusOK)
	})

	handler := StatusOverrideMiddleware(so)(inner)

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, "404", receivedHeader,
		"middleware should inject X-Mimikos-Status header")
}

func TestStatusOverrideMiddleware_OneShotConsumption(t *testing.T) {
	so := NewStatusOverrides()
	so.Set("GET", "/pets", 500)

	var headers []string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("X-Mimikos-Status"))

		w.WriteHeader(http.StatusOK)
	})

	handler := StatusOverrideMiddleware(so)(inner)

	// First request — should have override.
	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Second request — override consumed.
	req = httptest.NewRequest(http.MethodGet, "/pets", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.Len(t, headers, 2)
	assert.Equal(t, "500", headers[0], "first request should get override")
	assert.Empty(t, headers[1], "second request should not get override")
}

func TestStatusOverrideMiddleware_NonMatchingPassesThrough(t *testing.T) {
	so := NewStatusOverrides()
	so.Set("POST", "/pets", 422) // different endpoint

	var receivedHeader string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Mimikos-Status")

		w.WriteHeader(http.StatusOK)
	})

	handler := StatusOverrideMiddleware(so)(inner)

	req := httptest.NewRequest(http.MethodGet, "/pets", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Empty(t, receivedHeader, "non-matching request should not get header")
	assert.Equal(t, http.StatusOK, rec.Code)
}
