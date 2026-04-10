// Package router implements the HTTP router and scenario router for Mimikos.
// The HTTP router matches incoming requests to OpenAPI operations using
// net/http ServeMux (Go 1.22+). The scenario router selects the appropriate
// response scenario based on the behavior map and request validity.
package router

import (
	"bytes"
	"context"
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
	"github.com/mimikos-io/mimikos/internal/state"
	"github.com/mimikos-io/mimikos/internal/validator"
)

// maxBodySize is the maximum allowed request body size (10 MB).
const maxBodySize = 10 * 1024 * 1024

// contentTypeJSON is the standard JSON content type.
const contentTypeJSON = "application/json"

// methodsFor405 lists HTTP methods for which we register explicit 405 handlers
// when a path does not define them. This avoids method-less catch-all patterns
// that conflict with Go 1.22+ ServeMux's strict overlap detection.
//
// HEAD is excluded: ServeMux implicitly routes HEAD to GET handlers, so
// registering HEAD 405 handlers creates cross-path conflicts when a literal
// sibling has GET defined (e.g. HEAD /items/{id} vs GET /items/shared).
//
//nolint:gochecknoglobals
var methodsFor405 = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

type (
	// Handler serves mock HTTP responses by matching requests to OpenAPI
	// operations and orchestrating the runtime pipeline.
	Handler struct {
		mux       *http.ServeMux
		responder merrors.Responder
		logger    *slog.Logger
		strict    bool
		mode      model.OperatingMode
		store     state.Store // nil in deterministic mode
	}

	// discardHandler is a slog.Handler that discards all log output.
	discardHandler struct{}
)

// NewHandler creates a Handler that routes requests based on the given
// BehaviorMap. All dependencies are injected: validator for request
// validation, responder for error responses, generator for response data.
// When strict is true, response validation failures return 500 instead of
// logging a warning and sending the response anyway.
// In stateful mode, the handler consults the Store for CRUD state management.
// The store must be nil when mode is deterministic.
// If logger is nil, a no-op logger is used.
func NewHandler(
	behaviorMap *model.BehaviorMap,
	v validator.RequestValidator,
	responder merrors.Responder,
	gen *generator.DataGenerator,
	strict bool,
	logger *slog.Logger,
	mode model.OperatingMode,
	store state.Store,
) *Handler {
	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	mux := http.NewServeMux()
	h := &Handler{mux: mux, responder: responder, logger: logger, strict: strict, mode: mode, store: store}

	// Collect entries by path pattern for 405 handling.
	methodsByPath := make(map[string][]string)

	for _, entry := range behaviorMap.Entries() {
		pattern := entry.Method + " " + entry.PathPattern
		mux.HandleFunc(pattern, h.operationHandler(entry, v, gen))
		methodsByPath[entry.PathPattern] = append(methodsByPath[entry.PathPattern], entry.Method)
	}

	// Register per-method 405 handlers for undefined methods on each path.
	// We cannot use a method-less catch-all pattern because Go 1.22+ ServeMux
	// panics when a method-less literal path (e.g. /items/shared) overlaps
	// with a method-specific wildcard (e.g. PUT /items/{id}) — the literal is
	// more specific in path but broader in methods, which ServeMux considers
	// ambiguous. Registering explicit method-specific 405 handlers avoids this.
	for pathPattern, methods := range methodsByPath {
		sort.Strings(methods)

		defined := make(map[string]bool, len(methods))
		for _, m := range methods {
			defined[m] = true
		}

		allowedMethods := methods // capture for closure

		for _, m := range methodsFor405 {
			if defined[m] {
				continue
			}

			pattern := m + " " + pathPattern

			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				responder.MethodNotAllowed(w, r.Method, r.URL.Path, allowedMethods)
			})
		}
	}

	// Default catch-all: any request that doesn't match a registered path
	// gets an RFC 7807 404. This replaces the two-step Handler()+ServeHTTP()
	// approach so the mux's ServeHTTP sets path values on the request.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responder.NotFound(w, r.Method, r.URL.Path)
	})

	return h
}

// ServeHTTP implements http.Handler. Delegates directly to the internal
// ServeMux so that Go 1.22+ path values ({petId}, etc.) are populated on
// the request before handlers run. Unmatched routes fall through to the
// "/" catch-all which returns RFC 7807 404. Method-not-allowed cases are
// handled by the method-less catch-all handlers registered per path pattern.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// operationHandler returns an http.HandlerFunc for a single BehaviorEntry.
// It validates the request, then delegates to the stateful or deterministic
// handler based on the operating mode.
func (h *Handler) operationHandler(
	entry model.BehaviorEntry,
	v validator.RequestValidator,
	gen *generator.DataGenerator,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := h.validateRequest(w, r, v)
		if !ok {
			return
		}

		requestedStatus := r.Header.Get(statusHeader)

		// Stateful mode: when no explicit status is requested, delegate to
		// state-aware handler. Generic behaviors fall through to deterministic.
		if h.mode == model.ModeStateful && requestedStatus == "" {
			if h.handleStatefulMode(w, r, entry, gen, body) {
				return
			}
		}

		h.handleDeterministicMode(w, r, entry, gen, requestedStatus, body)
	}
}

// handleDeterministicMode generates a response from the schema using a fingerprint
// seed. The same request always produces the same response. When a media-type
// example is defined for the selected scenario, it is returned as-is — bypassing
// generation and response validation (the spec author wrote both schema and
// example, so validation would only catch spec defects, not Mimikos bugs).
func (h *Handler) handleDeterministicMode(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	requestedStatus string,
	body []byte,
) {
	scenario, err := SelectScenario(&entry, requestedStatus)
	if err != nil {
		h.responder.InvalidScenario(w, err.Error())

		return
	}

	// Media-type example: return spec-authored response directly.
	// Bypasses generation and response validation.
	// 204 No Content always has no body regardless of example.
	if scenario.Example != nil && scenario.StatusCode != http.StatusNoContent {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(scenario.StatusCode)

		//nolint:errchkjson // write failures (client disconnect) are unrecoverable
		_ = json.NewEncoder(w).Encode(scenario.Example)

		return
	}

	if scenario.Schema == nil && scenario.StatusCode != entry.SuccessCode {
		writeErrorFallback(w, scenario)

		return
	}

	seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)

	h.writeResponse(w, gen, scenario, seed)
}

// validateRequest performs body reading, content-type checking, and request
// validation. Returns the request body bytes and true on success, or writes
// an error response and returns false. On success, r.Body is replaced with
// a buffered reader for downstream consumers.
func (h *Handler) validateRequest(
	w http.ResponseWriter,
	r *http.Request,
	v validator.RequestValidator,
) ([]byte, bool) {
	body, err := readBody(r)
	if err != nil {
		writeProblem(w, http.StatusRequestEntityTooLarge, "Request body exceeds 10MB limit")

		return nil, false
	}

	if hasBody(r.Method) && len(body) > 0 {
		ct := r.Header.Get("Content-Type")
		if !isJSONContentType(ct) {
			h.responder.UnsupportedMediaType(w, ct)

			return nil, false
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

			return nil, false
		}

		h.responder.NotFound(w, r.Method, r.URL.Path)

		return nil, false
	}

	if len(validationErrs) > 0 {
		h.responder.ValidationError(w, validationErrs)

		return nil, false
	}

	return body, true
}

// writeResponse generates, validates, and writes the response for a matched
// request. For 204 No Content, no body is written. For nil schemas, an empty
// JSON object is returned as a sensible default.
//
// Response validation catches generator bugs where the produced data does not
// conform to the OpenAPI schema. In default mode, a warning is logged and the
// response is sent anyway. In strict mode, a 500 is returned instead.
func (h *Handler) writeResponse(
	w http.ResponseWriter,
	gen *generator.DataGenerator,
	scenario *SelectedScenario,
	seed int64,
) {
	if scenario.StatusCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	responseBody, ok := h.generateResponseBody(w, gen, scenario, seed)
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
func (h *Handler) generateResponseBody(
	w http.ResponseWriter,
	gen *generator.DataGenerator,
	scenario *SelectedScenario,
	seed int64,
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
		if h.strict {
			writeProblem(w, http.StatusInternalServerError,
				"response validation failed: "+valErr.Error())

			return nil, false
		}

		h.logger.Warn("generated response does not match schema",
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

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }
