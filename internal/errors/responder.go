// Package errors implements the error responder for Mimikos.
// It generates RFC 7807 Problem Details responses for invalid requests.
// Validation errors always use RFC 7807 because the diagnostic content
// (which fields failed, why) is the value — not a generated schema body.
package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mimikos-io/mimikos/internal/model"
)

const (
	// contentTypeProblemJSON is the RFC 7807 media type.
	contentTypeProblemJSON = "application/problem+json"

	// maxDetailLen caps interpolated user input in detail strings to prevent
	// oversized response bodies from absurd paths or header values.
	maxDetailLen = 200
)

type (
	// Responder generates HTTP error responses for invalid requests.
	Responder interface {
		// ValidationError writes a 400 response for one or more request
		// validation failures using RFC 7807 Problem Details.
		ValidationError(w http.ResponseWriter, errs []model.ValidationError)

		// NotFound writes a 404 response when no route matches the request.
		NotFound(w http.ResponseWriter, method, path string)

		// MethodNotAllowed writes a 405 response when the path matched but
		// the HTTP method is not supported. The allowed parameter lists the
		// methods the path does support, and is set as the Allow header
		// per RFC 9110 §15.5.6.
		MethodNotAllowed(w http.ResponseWriter, method, path string, allowed []string)

		// UnsupportedMediaType writes a 415 response when the request
		// Content-Type is not supported by the operation.
		UnsupportedMediaType(w http.ResponseWriter, contentType string)

		// NotAcceptable writes a 406 response when the server cannot produce
		// a response matching the request Accept header.
		NotAcceptable(w http.ResponseWriter, accept string)

		// InvalidScenario writes a 400 response when the X-Mimikos-Scenario
		// header requests a scenario that is not valid for the operation.
		InvalidScenario(w http.ResponseWriter, detail string)
	}

	// RFC 7807 types (unexported).

	// problemDetail is the RFC 7807 Problem Details response body.
	problemDetail struct {
		Type   string      `json:"type"`
		Title  string      `json:"title"`
		Status int         `json:"status"`
		Detail string      `json:"detail"`
		Errors []errorItem `json:"errors,omitempty"`
	}

	// errorItem is a single entry in the Problem Details errors extension array.
	errorItem struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}
)

// DefaultResponder implements [Responder] using RFC 7807 Problem Details
// for all error responses.
type DefaultResponder struct{}

// NewResponder creates a [DefaultResponder].
func NewResponder() *DefaultResponder {
	return &DefaultResponder{}
}

// ValidationError writes a 400 RFC 7807 response containing all validation
// failures. The errors are always included as-is — diagnostic content is
// the value for validation errors.
func (r *DefaultResponder) ValidationError(
	w http.ResponseWriter,
	errs []model.ValidationError,
) {
	var items []errorItem

	if len(errs) > 0 {
		items = make([]errorItem, len(errs))
		for i, e := range errs {
			items[i] = errorItem{Field: e.Field, Message: e.Message}
		}
	}

	writeProblem(w, http.StatusBadRequest, "Request validation failed", items)
}

// NotFound writes a 404 RFC 7807 response.
func (r *DefaultResponder) NotFound(w http.ResponseWriter, method, path string) {
	detail := fmt.Sprintf("No route matched %s %s", method, truncateDetail(path))
	writeProblem(w, http.StatusNotFound, detail, nil)
}

// MethodNotAllowed writes a 405 RFC 7807 response and sets the Allow header.
func (r *DefaultResponder) MethodNotAllowed(
	w http.ResponseWriter,
	method, path string,
	allowed []string,
) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))

	detail := fmt.Sprintf(
		"Method %s is not allowed for %s",
		method, truncateDetail(path),
	)

	writeProblem(w, http.StatusMethodNotAllowed, detail, nil)
}

// UnsupportedMediaType writes a 415 RFC 7807 response.
func (r *DefaultResponder) UnsupportedMediaType(w http.ResponseWriter, contentType string) {
	detail := "Content-Type " + truncateDetail(contentType) + " is not supported"
	writeProblem(w, http.StatusUnsupportedMediaType, detail, nil)
}

// NotAcceptable writes a 406 RFC 7807 response.
func (r *DefaultResponder) NotAcceptable(w http.ResponseWriter, accept string) {
	detail := "Cannot produce response matching Accept: " + truncateDetail(accept)
	writeProblem(w, http.StatusNotAcceptable, detail, nil)
}

// InvalidScenario writes a 400 RFC 7807 response for invalid scenario requests.
func (r *DefaultResponder) InvalidScenario(w http.ResponseWriter, detail string) {
	writeProblem(w, http.StatusBadRequest, truncateDetail(detail), nil)
}

// --- helpers ---

// writeProblem writes a complete RFC 7807 Problem Details response.
func writeProblem(w http.ResponseWriter, status int, detail string, errors []errorItem) {
	pd := problemDetail{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Errors: errors,
	}

	writeJSON(w, contentTypeProblemJSON, status, pd)
}

// writeJSON encodes body as JSON and writes it to w with the given content
// type and status code. Write errors are silently discarded — if the client
// disconnected, there is nothing useful to do.
func writeJSON(w http.ResponseWriter, ct string, status int, body any) {
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(status)
	//nolint:errchkjson // write failures (client disconnect) are unrecoverable
	_ = json.NewEncoder(w).Encode(body)
}

// truncateDetail caps a user-supplied string at maxDetailLen characters to
// prevent oversized response bodies from absurd paths or header values.
func truncateDetail(s string) string {
	if len(s) > maxDetailLen {
		return s[:maxDetailLen] + "..."
	}

	return s
}
