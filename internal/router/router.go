// Package router implements the HTTP router and scenario router for Mimikos.
// The HTTP router matches incoming requests to OpenAPI operations using chi.
// The scenario router selects the appropriate response scenario based on the
// behavior map and request validity.
package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mimikos-io/mimikos/internal/builder"
	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
	"github.com/mimikos-io/mimikos/internal/validator"
)

const (
	// maxBodySize is the maximum allowed request body size (10 MB).
	maxBodySize = 10 * 1024 * 1024

	// contentTypeJSON is the standard JSON content type.
	contentTypeJSON = "application/json"
)

// httpMethods lists the HTTP methods checked when building the Allow header
// for 405 responses. HEAD is included because chi implicitly routes HEAD
// to GET handlers.
//
//nolint:gochecknoglobals
var httpMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

type (
	// Handler serves mock HTTP responses by matching requests to OpenAPI
	// operations and orchestrating the runtime pipeline.
	Handler struct {
		router    chi.Router
		responder merrors.Responder
		logger    *slog.Logger
		strict    bool
		mode      model.OperatingMode
		store     state.Store // nil in deterministic mode
	}

	// discardHandler is a slog.Handler that discards all log output.
	discardHandler struct{}
)

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// NewHandler creates a Handler that routes requests based on the given
// BehaviorMap. All dependencies are injected: validator for request
// validation, responder for error responses, generator for response data.
// When strict is true, response validation failures return 500 instead of
// logging a warning and sending the response anyway.
// In stateful mode, the handler consults the Store for CRUD state management.
// The store must be nil when mode is deterministic.
//
// Failed entries are operations that panicked during the startup pipeline.
// Each gets a placeholder handler that returns an actionable RFC 7807 error
// instead of falling through to 404.
//
// If logger is nil, a no-op logger is used.
func NewHandler(
	behaviorMap *model.BehaviorMap,
	failed []builder.FailedEntry,
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

	r := chi.NewRouter()
	h := &Handler{router: r, responder: responder, logger: logger, strict: strict, mode: mode, store: store}

	r.Use(h.recoverer) // catch panics in handlers → RFC 7807 500

	for _, entry := range behaviorMap.Entries() {
		r.MethodFunc(entry.Method, entry.PathPattern, h.operationHandler(entry, v, gen))
	}

	// Register placeholder handlers for entries that panicked at startup.
	// Without this, failed endpoints fall through to 404, misleading the
	// developer into thinking their spec path is wrong.
	for _, f := range failed {
		detail := fmt.Sprintf(
			"This endpoint failed to register at startup: %s. Check the startup log for details.",
			f.Error,
		)
		r.MethodFunc(f.Method, f.PathPattern, func(w http.ResponseWriter, _ *http.Request) {
			writeProblem(w, http.StatusInternalServerError, detail)
		})
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		responder.NotFound(w, r.Method, r.URL.Path)
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		responder.MethodNotAllowed(w, req.Method, req.URL.Path, allowedMethods(r, req.URL.Path))
	})

	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// recoverer is chi middleware that catches panics in handlers and returns
// an RFC 7807 500 response instead of crashing the connection.
func (h *Handler) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				h.logger.Error("handler panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"error", rec,
					"stack", string(debug.Stack()),
				)
				writeProblem(w, http.StatusInternalServerError,
					fmt.Sprintf("Unexpected error processing %s %s: %v", r.Method, r.URL.Path, rec))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// allowedMethods returns the HTTP methods that the router can serve for
// the given path. Used to populate the Allow header on 405 responses.
func allowedMethods(r chi.Router, path string) []string {
	var methods []string

	for _, method := range httpMethods {
		if r.Match(chi.NewRouteContext(), method, path) {
			methods = append(methods, method)
		}
	}

	return methods
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
		// Strict mode: request schema degradation blocks body-bearing methods.
		// The developer opted into correctness, but we cannot validate.
		if h.strict && entry.DegradedRequestSchema != "" && hasBody(r.Method) {
			writeProblem(w, http.StatusInternalServerError,
				"Request body validation unavailable — schema failed to compile at startup: "+
					entry.DegradedRequestSchema)

			return
		}

		// validateRequest MUST run before the mode branch below. Stateful
		// handlers (handleCreate, handleUpdate) assume a valid body —
		// parseRequestBody silently treats an empty body as {}, which would
		// create phantom resources if the required-body check were skipped.
		body, ok := h.validateRequest(w, r, v, entry.BodyRequired)
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

	// Response schema degraded: the spec defined a schema for this status code
	// but it failed to compile at startup. Return RFC 7807 instead of empty data
	// — unless a media-type example can fill in (checked below).
	if entry.DegradedResponseSchema != "" && scenario.StatusCode == entry.SuccessCode &&
		scenario.Schema == nil && scenario.Example == nil {
		writeProblem(w, http.StatusInternalServerError,
			"Response generation unavailable — schema failed to compile at startup: "+
				entry.DegradedResponseSchema)

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
//
// When bodyRequired is true, an empty request body is rejected with a clear
// validation error before content-type or schema validation runs.
func (h *Handler) validateRequest(
	w http.ResponseWriter,
	r *http.Request,
	v validator.RequestValidator,
	bodyRequired bool,
) ([]byte, bool) {
	body, err := readBody(r)
	if err != nil {
		writeProblem(w, http.StatusRequestEntityTooLarge, "Request body exceeds 10MB limit")

		return nil, false
	}

	// Required body check — before content-type and schema validation.
	// Short-circuits with a clear message instead of falling through to
	// libopenapi-validator which produces confusing content-type errors.
	if bodyRequired && len(body) == 0 {
		h.responder.ValidationError(w, []model.ValidationError{
			{Field: "", Message: "Request body is required"},
		})

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
		// Defensive: sentinel errors should not fire here since the router
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
