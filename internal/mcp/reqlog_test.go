package mcp

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RequestLog tests ---

func TestRequestLog_RecordAndRecent(t *testing.T) {
	rl := NewRequestLog(5)

	rl.Record(Entry{Method: "GET", Path: "/a", StatusCode: 200})
	rl.Record(Entry{Method: "POST", Path: "/b", StatusCode: 201})
	rl.Record(Entry{Method: "DELETE", Path: "/c", StatusCode: 204})

	entries := rl.Recent(10)

	require.Len(t, entries, 3)
	// Newest first.
	assert.Equal(t, "DELETE", entries[0].Method)
	assert.Equal(t, "/c", entries[0].Path)
	assert.Equal(t, 204, entries[0].StatusCode)

	assert.Equal(t, "POST", entries[1].Method)
	assert.Equal(t, "/b", entries[1].Path)

	assert.Equal(t, "GET", entries[2].Method)
	assert.Equal(t, "/a", entries[2].Path)
}

func TestRequestLog_RecentRespectsLimit(t *testing.T) {
	rl := NewRequestLog(10)

	rl.Record(Entry{Method: "GET", Path: "/a", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/b", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/c", StatusCode: 200})

	entries := rl.Recent(2)

	require.Len(t, entries, 2)
	assert.Equal(t, "/c", entries[0].Path, "newest first")
	assert.Equal(t, "/b", entries[1].Path)
}

func TestRequestLog_OverflowEvictsOldest(t *testing.T) {
	rl := NewRequestLog(3)

	rl.Record(Entry{Method: "GET", Path: "/a", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/b", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/c", StatusCode: 200})
	// Overflow — /a should be evicted.
	rl.Record(Entry{Method: "GET", Path: "/d", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/e", StatusCode: 200})

	entries := rl.Recent(10)

	require.Len(t, entries, 3, "should cap at capacity")
	assert.Equal(t, "/e", entries[0].Path)
	assert.Equal(t, "/d", entries[1].Path)
	assert.Equal(t, "/c", entries[2].Path)
}

func TestRequestLog_RecentLimitGreaterThanCount(t *testing.T) {
	rl := NewRequestLog(100)

	rl.Record(Entry{Method: "GET", Path: "/only", StatusCode: 200})

	entries := rl.Recent(50)

	require.Len(t, entries, 1, "should return all when limit > count")
	assert.Equal(t, "/only", entries[0].Path)
}

func TestRequestLog_RecentEmpty(t *testing.T) {
	rl := NewRequestLog(5)

	entries := rl.Recent(10)

	assert.Empty(t, entries)
}

func TestRequestLog_Reset(t *testing.T) {
	rl := NewRequestLog(5)

	rl.Record(Entry{Method: "GET", Path: "/a", StatusCode: 200})
	rl.Record(Entry{Method: "GET", Path: "/b", StatusCode: 200})

	rl.Reset()

	entries := rl.Recent(10)
	assert.Empty(t, entries, "reset should clear all entries")

	// Should be usable after reset.
	rl.Record(Entry{Method: "POST", Path: "/c", StatusCode: 201})

	entries = rl.Recent(10)
	require.Len(t, entries, 1)
	assert.Equal(t, "/c", entries[0].Path)
}

func TestRequestLog_PreservesTimestamp(t *testing.T) {
	rl := NewRequestLog(5)

	ts := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	rl.Record(Entry{
		Method:     "GET",
		Path:       "/ts",
		StatusCode: 200,
		Timestamp:  ts,
	})

	entries := rl.Recent(1)
	require.Len(t, entries, 1)
	assert.Equal(t, ts, entries[0].Timestamp)
}

func TestRequestLog_PreservesValidationErrors(t *testing.T) {
	rl := NewRequestLog(5)

	rl.Record(Entry{
		Method:           "POST",
		Path:             "/bad",
		StatusCode:       400,
		ValidationErrors: []string{"missing field: name", "invalid type: age"},
	})

	entries := rl.Recent(1)
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"missing field: name", "invalid type: age"},
		entries[0].ValidationErrors)
}

func TestRequestLog_ConcurrentRecordAndRecent(t *testing.T) {
	rl := NewRequestLog(50)

	var wg sync.WaitGroup

	// Concurrent writers.
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			rl.Record(Entry{
				Method:     "GET",
				Path:       "/concurrent",
				StatusCode: 200 + n,
			})
		}(i)
	}

	// Concurrent readers.
	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_ = rl.Recent(10)
		}()
	}

	wg.Wait()

	// After all writers complete, we should have exactly 20 entries.
	entries := rl.Recent(100)
	assert.Len(t, entries, 20)
}

func TestRequestLog_RecentReturnsDefensiveCopy(t *testing.T) {
	rl := NewRequestLog(5)

	rl.Record(Entry{Method: "GET", Path: "/a", StatusCode: 200})

	entries := rl.Recent(10)
	entries[0].Path = "/mutated"

	// Original should be unaffected.
	fresh := rl.Recent(10)
	assert.Equal(t, "/a", fresh[0].Path, "Recent should return a copy")
}

// --- RequestLog middleware tests ---

func TestRequestLogMiddleware_CapturesMethodPathStatus(t *testing.T) {
	rl := NewRequestLog(10)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	handler := RequestLogMiddleware(rl)(inner)

	req := httptest.NewRequest(http.MethodPost, "/pets", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Response should pass through unchanged.
	assert.Equal(t, http.StatusCreated, rec.Code)

	entries := rl.Recent(10)
	require.Len(t, entries, 1)
	assert.Equal(t, "POST", entries[0].Method)
	assert.Equal(t, "/pets", entries[0].Path)
	assert.Equal(t, http.StatusCreated, entries[0].StatusCode)
	assert.False(t, entries[0].Timestamp.IsZero(), "timestamp should be set")
}

func TestRequestLogMiddleware_DefaultStatusIs200(t *testing.T) {
	rl := NewRequestLog(10)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No explicit WriteHeader — defaults to 200.
		_, _ = w.Write([]byte("ok"))
	})

	handler := RequestLogMiddleware(rl)(inner)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	entries := rl.Recent(10)
	require.Len(t, entries, 1)
	assert.Equal(t, http.StatusOK, entries[0].StatusCode)
}

func TestRequestLogMiddleware_TransparentToInnerHandler(t *testing.T) {
	rl := NewRequestLog(10)

	var calledWith *http.Request

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledWith = r

		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("body"))
	})

	handler := RequestLogMiddleware(rl)(inner)

	req := httptest.NewRequest(http.MethodPut, "/resource/42", nil)
	req.Header.Set("Authorization", "Bearer tok")

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Inner handler received the original request.
	require.NotNil(t, calledWith)
	assert.Equal(t, "Bearer tok", calledWith.Header.Get("Authorization"))

	// Response headers and body pass through.
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "value", rec.Header().Get("X-Custom"))
	assert.Equal(t, "body", rec.Body.String())
}

func TestRequestLogMiddleware_MultipleRequests(t *testing.T) {
	rl := NewRequestLog(10)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLogMiddleware(rl)(inner)

	for _, method := range []string{"GET", "POST", "DELETE"} {
		req := httptest.NewRequest(method, "/items", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	entries := rl.Recent(10)
	require.Len(t, entries, 3)
	// Newest first.
	assert.Equal(t, "DELETE", entries[0].Method)
	assert.Equal(t, "POST", entries[1].Method)
	assert.Equal(t, "GET", entries[2].Method)
}
