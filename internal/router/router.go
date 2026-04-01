// Package router implements the HTTP router and scenario router for Mimikos.
// The HTTP router matches incoming requests to OpenAPI operations using
// net/http ServeMux (Go 1.22+). The scenario router selects the appropriate
// response scenario based on the behavior map and request validity.
package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/validator"
)

// maxBodySize is the maximum allowed request body size (10 MB).
const maxBodySize = 10 * 1024 * 1024

// contentTypeJSON is the standard JSON content type.
const contentTypeJSON = "application/json"

// Handler serves mock HTTP responses by matching requests to OpenAPI
// operations and orchestrating the runtime pipeline.
type Handler struct {
	mux       *http.ServeMux
	responder merrors.Responder
	strict    bool
}

// NewHandler creates a Handler that routes requests based on the given
// BehaviorMap. All dependencies are injected: validator for request
// validation, responder for error responses, generator for response data.
// When strict is true, response validation failures return 500 instead of
// logging a warning and sending the response anyway.
func NewHandler(
	behaviorMap *model.BehaviorMap,
	v validator.RequestValidator,
	responder merrors.Responder,
	gen *generator.DataGenerator,
	strict bool,
) *Handler {
	mux := http.NewServeMux()
	h := &Handler{mux: mux, responder: responder, strict: strict}

	// Collect entries by path pattern for 405 handling.
	methodsByPath := make(map[string][]string)

	for _, entry := range behaviorMap.Entries() {
		pattern := entry.Method + " " + entry.PathPattern
		mux.HandleFunc(pattern, h.operationHandler(entry, v, gen))
		methodsByPath[entry.PathPattern] = append(methodsByPath[entry.PathPattern], entry.Method)
	}

	// Register method-less catch-all per path pattern for 405 responses.
	for pathPattern, methods := range methodsByPath {
		sort.Strings(methods)

		mux.HandleFunc(pathPattern, func(w http.ResponseWriter, r *http.Request) {
			responder.MethodNotAllowed(w, r.Method, r.URL.Path, methods)
		})
	}

	return h
}

// ServeHTTP implements http.Handler. Unmatched routes produce RFC 7807 404.
//
// Invariant: pattern=="" means no path matched. Method-not-allowed cases are
// handled by the catch-all handlers registered per path pattern in NewHandler,
// so they never reach this branch.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, pattern := h.mux.Handler(r)
	if pattern == "" {
		h.responder.NotFound(w, r.Method, r.URL.Path)

		return
	}

	handler.ServeHTTP(w, r)
}

// operationHandler returns an http.HandlerFunc for a single BehaviorEntry.
// It executes the per-request runtime pipeline:
//
//  1. Read body (with size limit)
//  2. Content-type check (for methods with body)
//  3. Request validation
//  4. Fingerprint
//  5. Scenario selection
//  6. Data generation
//  7. Response writing
func (h *Handler) operationHandler(
	entry model.BehaviorEntry,
	v validator.RequestValidator,
	gen *generator.DataGenerator,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read body with size limit.
		body, err := readBody(r)
		if err != nil {
			writeProblem(w, http.StatusRequestEntityTooLarge, "Request body exceeds 10MB limit")

			return
		}

		if hasBody(r.Method) && len(body) > 0 {
			ct := r.Header.Get("Content-Type")
			if !isJSONContentType(ct) {
				h.responder.UnsupportedMediaType(w, ct)

				return
			}
		}

		// Replace body so the validator can read it.
		r.Body = io.NopCloser(bytes.NewReader(body))

		validationErrs, valErr := v.Validate(r)
		if valErr != nil {
			// Defensive: sentinel errors should not fire here since ServeMux
			// already matched the route, but handle them correctly if they do.
			if errors.Is(valErr, validator.ErrOperationNotFound) {
				h.responder.MethodNotAllowed(w, r.Method, r.URL.Path, nil)

				return
			}

			h.responder.NotFound(w, r.Method, r.URL.Path)

			return
		}

		if len(validationErrs) > 0 {
			h.responder.ValidationError(w, validationErrs)

			return
		}

		requestedStatus := r.Header.Get(statusHeader)

		scenario, err := SelectScenario(&entry, requestedStatus)
		if err != nil {
			h.responder.InvalidScenario(w, err.Error())

			return
		}

		if scenario.Schema == nil && scenario.StatusCode != entry.SuccessCode {
			// Error status with no spec-defined schema — use RFC 7807 fallback.
			writeErrorFallback(w, scenario)

			return
		}

		seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)

		writeResponse(w, gen, scenario, seed, h.strict)
	}
}

// writeResponse generates, validates, and writes the response for a matched
// request. For 204 No Content, no body is written. For nil schemas, an empty
// JSON object is returned as a sensible default.
//
// Response validation catches generator bugs where the produced data does not
// conform to the OpenAPI schema. In default mode, a warning is logged and the
// response is sent anyway. In strict mode, a 500 is returned instead.
func writeResponse(
	w http.ResponseWriter,
	gen *generator.DataGenerator,
	scenario *SelectedScenario,
	seed int64,
	strict bool,
) {
	if scenario.StatusCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	responseBody, ok := generateResponseBody(w, gen, scenario, seed, strict)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(scenario.StatusCode)

	//nolint:errchkjson // write failures (client disconnect) are unrecoverable
	_ = json.NewEncoder(w).Encode(responseBody)
}

// readBody reads the request body up to maxBodySize. Returns an error if
// the body exceeds the limit.
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	limited := http.MaxBytesReader(nil, r.Body, maxBodySize)

	defer func() { _ = limited.Close() }()

	return io.ReadAll(limited)
}

// hasBody returns true for HTTP methods that typically have a request body.
func hasBody(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

// isJSONContentType returns true if the content type is JSON or a JSON variant
// (e.g., application/vnd.api+json, application/merge-patch+json).
func isJSONContentType(ct string) bool {
	// Strip parameters (e.g., charset=utf-8).
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}

	ct = strings.TrimSpace(strings.ToLower(ct))

	if ct == contentTypeJSON {
		return true
	}

	// Accept application/*+json variants (RFC 6838 §4.2.8).
	return strings.HasPrefix(ct, "application/") && strings.HasSuffix(ct, "+json")
}

// writeErrorFallback writes an RFC 7807 response for error status codes that
// have no spec-defined error schema. This is the fallback when the OpenAPI spec
// does not define a response body for the requested error status code.
func writeErrorFallback(w http.ResponseWriter, scenario *SelectedScenario) {
	writeProblem(w, scenario.StatusCode,
		"Simulated "+http.StatusText(scenario.StatusCode)+" response")
}

// generateResponseBody produces the response payload using the generator and
// validates it against the schema. Returns the body and true on success, or
// writes an error response and returns false if generation fails or strict
// validation rejects the output.
func generateResponseBody(
	w http.ResponseWriter,
	gen *generator.DataGenerator,
	scenario *SelectedScenario,
	seed int64,
	strict bool,
) (any, bool) {
	if scenario.Schema == nil || scenario.Schema.Schema == nil {
		return map[string]any{}, true
	}

	responseBody, err := gen.Generate(scenario.Schema.Schema, seed)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Failed to generate response")

		return nil, false
	}

	if valErr := scenario.Schema.Validate(responseBody); valErr != nil {
		if strict {
			writeProblem(w, http.StatusInternalServerError,
				"response validation failed: "+valErr.Error())

			return nil, false
		}

		slog.Warn("generated response does not match schema",
			"schema", scenario.Schema.Name,
			"error", valErr.Error(),
		)
	}

	return responseBody, true
}

// writeProblem writes a minimal RFC 7807 response for infrastructure errors
// (body too large, generation failure) that don't go through the Responder.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	pd := map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	}
	//nolint:errchkjson // write failures (client disconnect) are unrecoverable
	_ = json.NewEncoder(w).Encode(pd)
}
