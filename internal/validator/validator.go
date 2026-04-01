// Package validator implements request validation against an OpenAPI
// specification. It wraps libopenapi-validator to validate path parameters,
// query parameters, headers, and request bodies, mapping all failures to
// [model.ValidationError] for consumption by the error responder.
package validator

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pb33f/libopenapi"
	liboapivalidator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
	liberrors "github.com/pb33f/libopenapi-validator/errors"
	"github.com/pb33f/libopenapi-validator/helpers"

	"github.com/mimikos-io/mimikos/internal/model"
)

// Sentinel errors for validator failures.
var (
	// ErrValidatorInit is returned when the libopenapi-validator cannot be constructed.
	ErrValidatorInit = errors.New("validator: failed to initialize")

	// ErrPathNotFound is returned when the request path does not match any path in the spec.
	ErrPathNotFound = errors.New("validator: path not found in spec")

	// ErrOperationNotFound is returned when the path matches but the HTTP method is not defined.
	ErrOperationNotFound = errors.New("validator: operation not found for path")
)

// RequestValidator validates HTTP requests against an OpenAPI specification.
type RequestValidator interface {
	// Validate checks the request against the spec.
	// Returns validation errors found in the request, or an empty slice if valid.
	// Returns a sentinel error ([ErrPathNotFound], [ErrOperationNotFound]) if the
	// request does not match any spec route — distinct from validation failures.
	Validate(r *http.Request) ([]model.ValidationError, error)
}

// LibopenAPIValidator implements [RequestValidator] using libopenapi-validator.
type LibopenAPIValidator struct {
	v liboapivalidator.Validator
}

// NewLibopenAPIValidator creates a request validator from a parsed [libopenapi.Document].
// The CLI creates the document once and shares it with the parser and validator.
// Returns an error if the validator cannot be constructed.
func NewLibopenAPIValidator(doc libopenapi.Document) (*LibopenAPIValidator, error) {
	v, errs := liboapivalidator.NewValidator(
		doc,
		config.WithoutSecurityValidation(),
		config.WithFormatAssertions(),
	)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", ErrValidatorInit, errors.Join(errs...))
	}

	return &LibopenAPIValidator{v: v}, nil
}

// Validate checks the request against the OpenAPI spec. It collects all
// validation errors (not fail-fast) and maps them to [model.ValidationError].
//
// If the request path is not found in the spec, [ErrPathNotFound] is returned.
// If the path matches but the method is not defined, [ErrOperationNotFound] is returned.
// These are infrastructure errors, not validation failures.
func (lv *LibopenAPIValidator) Validate(r *http.Request) ([]model.ValidationError, error) {
	valid, libErrs := lv.v.ValidateHttpRequestSync(r)
	if valid {
		return nil, nil
	}

	// Check for routing-level errors first.
	for _, le := range libErrs {
		if le.IsPathMissingError() {
			return nil, ErrPathNotFound
		}

		if le.IsOperationMissingError() {
			return nil, ErrOperationNotFound
		}
	}

	return mapErrors(libErrs), nil
}

// mapErrors converts libopenapi-validator errors to model.ValidationError.
// It flattens the two-level hierarchy (ValidationError → SchemaValidationFailure)
// into a single flat slice.
func mapErrors(libErrs []*liberrors.ValidationError) []model.ValidationError {
	var result []model.ValidationError

	for _, le := range libErrs {
		if le == nil {
			continue
		}

		switch le.ValidationType {
		case helpers.ParameterValidation:
			result = append(result, mapParameterError(le)...)
		case helpers.RequestBodyValidation:
			result = append(result, mapRequestBodyError(le)...)
		default:
			// Known types that reach here: "request" (content-type from
			// requests/validate_body.go), "security" (disabled via config),
			// "path" (handled above as sentinel). All produce a generic error.
			result = append(result, model.ValidationError{
				Field:   le.ParameterName,
				Message: firstNonEmpty(le.Message, le.Reason),
			})
		}
	}

	return result
}

// mapParameterError maps a parameter validation error (path, query, header, cookie).
func mapParameterError(le *liberrors.ValidationError) []model.ValidationError {
	fieldPrefix := le.ValidationSubType + "." + le.ParameterName

	if len(le.SchemaValidationErrors) == 0 {
		return []model.ValidationError{
			{Field: fieldPrefix, Message: firstNonEmpty(le.Message, le.Reason)},
		}
	}

	errs := make([]model.ValidationError, 0, len(le.SchemaValidationErrors))

	for _, sve := range le.SchemaValidationErrors {
		errs = append(errs, model.ValidationError{
			Field:   fieldPrefix,
			Message: firstNonEmpty(sve.Reason, le.Message),
		})
	}

	return errs
}

// mapRequestBodyError maps a request body validation error.
func mapRequestBodyError(le *liberrors.ValidationError) []model.ValidationError {
	if len(le.SchemaValidationErrors) == 0 {
		return []model.ValidationError{
			{Field: "", Message: firstNonEmpty(le.Message, le.Reason)},
		}
	}

	errs := make([]model.ValidationError, 0, len(le.SchemaValidationErrors))

	for _, sve := range le.SchemaValidationErrors {
		field := fieldPathToJSONPointer(sve.FieldPath)

		errs = append(errs, model.ValidationError{
			Field:   field,
			Message: firstNonEmpty(sve.Reason, le.Message),
		})
	}

	return errs
}

// fieldPathToJSONPointer converts a JSONPath-style field path from
// libopenapi-validator to a JSON pointer. For example:
//
//	"$.name"      → "/name"
//	"$.body.name" → "/body/name"
//	""            → ""
func fieldPathToJSONPointer(fp string) string {
	if fp == "" || fp == "$" {
		return ""
	}

	// Strip leading "$." prefix.
	trimmed := strings.TrimPrefix(fp, "$.")
	if trimmed == "" {
		return ""
	}

	// Replace dots with slashes and add leading slash.
	return "/" + strings.ReplaceAll(trimmed, ".", "/")
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}

	return ""
}
