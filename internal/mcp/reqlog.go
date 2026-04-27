package mcp

import (
	"net/http"
	"sync"
	"time"
)

type (
	// Entry records a single HTTP request/response observed by the mock server.
	Entry struct {
		// Method is the HTTP method (GET, POST, etc.).
		Method string

		// Path is the request URL path.
		Path string

		// StatusCode is the HTTP status code returned.
		StatusCode int

		// Timestamp is when the request was received.
		Timestamp time.Time

		// ValidationErrors contains any request validation error messages.
		ValidationErrors []string
	}

	// RequestLog is a goroutine-safe fixed-capacity ring buffer that records the
	// most recent HTTP request entries. When the buffer is full, the oldest entry
	// is overwritten.
	RequestLog struct {
		mu      sync.Mutex
		entries []Entry
		head    int
		count   int
		cap     int
	}

	// statusRecorder wraps http.ResponseWriter to capture the status code written
	// by the inner handler. If WriteHeader is never called, the status defaults to
	// 200 (matching net/http behavior).
	statusRecorder struct {
		http.ResponseWriter

		code    int
		written bool
	}
)

// NewRequestLog creates a ring buffer with the given capacity. Capacity must
// be at least 1; values below 1 are clamped.
func NewRequestLog(capacity int) *RequestLog {
	if capacity < 1 {
		capacity = 1
	}

	return &RequestLog{
		entries: make([]Entry, capacity),
		cap:     capacity,
	}
}

// Record adds an entry to the ring buffer. If the buffer is full, the oldest
// entry is overwritten.
func (rl *RequestLog) Record(e Entry) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.entries[rl.head] = e
	rl.head = (rl.head + 1) % rl.cap

	if rl.count < rl.cap {
		rl.count++
	}
}

// Recent returns up to limit entries in newest-first order. The returned slice
// is a defensive copy — callers may modify it without affecting the buffer.
func (rl *RequestLog) Recent(limit int) []Entry {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	n := rl.count
	if limit < n {
		n = limit
	}

	if n == 0 {
		return nil
	}

	result := make([]Entry, n)

	for i := range n {
		// Walk backwards from head-1 (the most recent entry).
		idx := (rl.head - 1 - i + rl.cap) % rl.cap
		result[i] = rl.entries[idx]
	}

	return result
}

// Reset clears all entries from the buffer.
func (rl *RequestLog) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.head = 0
	rl.count = 0
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.written {
		sr.code = code
		sr.written = true
	}

	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.code = http.StatusOK
		sr.written = true
	}

	return sr.ResponseWriter.Write(b)
}

// RequestLogMiddleware returns middleware that records each request's method,
// path, and response status code to the given RequestLog. It is transparent to
// the wrapped handler — request and response pass through unmodified.
func RequestLogMiddleware(rl *RequestLog) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}

			next.ServeHTTP(rec, r)

			rl.Record(Entry{
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: rec.code,
				Timestamp:  timeNow(),
			})
		})
	}
}
