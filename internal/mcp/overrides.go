package mcp

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// StatusOverrides is a goroutine-safe map for one-shot HTTP status code
// overrides. When a status override is set for a (method, path) pair, the next
// matching request will receive that status code, and the override is removed.
type StatusOverrides struct {
	mu        sync.Mutex
	overrides map[string]int
}

// NewStatusOverrides creates an empty override map.
func NewStatusOverrides() *StatusOverrides {
	return &StatusOverrides{
		overrides: make(map[string]int),
	}
}

// overrideKey builds the map key from method and path.
func overrideKey(method, path string) string {
	return fmt.Sprintf("%s %s", method, path)
}

// Set stores a status code override for the given endpoint. If an override
// already exists for this endpoint, it is replaced.
func (so *StatusOverrides) Set(method, path string, statusCode int) {
	so.mu.Lock()
	defer so.mu.Unlock()

	so.overrides[overrideKey(method, path)] = statusCode
}

// Consume returns and removes the status code override for the given endpoint.
// Returns (0, false) if no override is set.
func (so *StatusOverrides) Consume(method, path string) (int, bool) {
	so.mu.Lock()
	defer so.mu.Unlock()

	key := overrideKey(method, path)

	code, ok := so.overrides[key]
	if ok {
		delete(so.overrides, key)
	}

	return code, ok
}

// StatusOverrideMiddleware returns middleware that checks for a status override
// on each incoming request. If one exists, it injects the X-Mimikos-Status
// header into the request so the router produces the overridden response, and
// removes the override (one-shot semantics).
func StatusOverrideMiddleware(so *StatusOverrides) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code, ok := so.Consume(r.Method, r.URL.Path)
			if ok {
				r.Header.Set("X-Mimikos-Status", strconv.Itoa(code))
			}

			next.ServeHTTP(w, r)
		})
	}
}
