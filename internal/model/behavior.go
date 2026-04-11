// Package model defines the shared domain types used across all mimikos
// components, including BehaviorType, BehaviorEntry, BehaviorMap,
// and CompiledSchema.
package model

import (
	"errors"
	"fmt"
)

type (
	// BehaviorType represents the inferred CRUD behavior of an API operation.
	BehaviorType string

	// BehaviorEntry describes the inferred behavior of a single API operation.
	BehaviorEntry struct {
		// OperationID is the OpenAPI operationId, if present.
		OperationID string

		// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE).
		Method string

		// PathPattern is the OpenAPI path template (e.g., "/pets/{petId}").
		PathPattern string

		// Type is the inferred CRUD behavior.
		Type BehaviorType

		// SuccessCode is the HTTP status code for the success scenario (e.g., 200, 201).
		SuccessCode int

		// RequestSchema is the compiled schema for the request body, or nil if none.
		RequestSchema *CompiledSchema

		// ResponseSchemas maps HTTP status codes to their compiled response schemas.
		ResponseSchemas map[int]*CompiledSchema

		// Source indicates how this classification was determined.
		Source Source

		// Confidence is the classifier's confidence in the behavior type (0.0–1.0).
		Confidence float64

		// WrapperKey is the single-property wrapper key for this operation's
		// success response schema. Empty string means flat (no wrapper).
		// Detected at startup for create/fetch/update behaviors.
		// Example: "data" for Asana's {data: {resource}} pattern.
		WrapperKey string

		// ListArrayKey is the property name containing the array of resources
		// in a list response schema. Empty string means bare array (top-level
		// type: array). Detected at startup for list behaviors.
		// Example: "data" for Asana, "items" for Spotify, "results" for Notion.
		ListArrayKey string

		// IDFieldHint is the body field name expected to hold the resource ID.
		// Computed at startup by correlating the fetch variant's path parameter
		// with suffix-stripping. Empty means no hint — fall through to "id"
		// convention.
		// Example: "gid" for Asana (from fetch path /projects/{project_gid}).
		IDFieldHint string

		// BodyRequired is true when the OpenAPI spec marks the request body
		// as required (requestBody.required: true). Used by the router to
		// reject requests with missing bodies before delegating to the validator.
		BodyRequired bool

		// ResponseExamples maps HTTP status codes to complete media-type example
		// values extracted from the OpenAPI spec. When present, the router can
		// return the example directly instead of generating data from the schema.
		// Key 0 represents the default response example.
		// Nil map means no media-type examples were defined.
		ResponseExamples map[int]any
	}
)

// Sentinel errors for behavior validation.
var (
	// ErrInvalidBehaviorType is returned when parsing an unknown behavior type string.
	ErrInvalidBehaviorType = errors.New("invalid behavior type")
	// ErrInvalidSource is returned when a source string is not recognized.
	ErrInvalidSource = errors.New("invalid source")
	// ErrMissingMethod is returned when a BehaviorEntry has no HTTP method.
	ErrMissingMethod = errors.New("behavior entry: method is required")
	// ErrMissingPathPattern is returned when a BehaviorEntry has no path pattern.
	ErrMissingPathPattern = errors.New("behavior entry: path pattern is required")
	// ErrMissingSuccessCode is returned when a BehaviorEntry has no success status code.
	ErrMissingSuccessCode = errors.New("behavior entry: success code is required")
	// ErrInvalidConfidence is returned when confidence is outside [0.0, 1.0].
	ErrInvalidConfidence = errors.New("behavior entry: confidence must be between 0.0 and 1.0")

	//nolint:gochecknoglobals // immutable lookup table for behavior type validation
	validBehaviorTypes = map[BehaviorType]struct{}{
		BehaviorCreate:  {},
		BehaviorFetch:   {},
		BehaviorList:    {},
		BehaviorUpdate:  {},
		BehaviorDelete:  {},
		BehaviorGeneric: {},
	}
)

const (
	// BehaviorCreate indicates a resource creation operation (typically POST).
	BehaviorCreate BehaviorType = "create"
	// BehaviorFetch indicates a single resource retrieval (typically GET /{id}).
	BehaviorFetch BehaviorType = "fetch"
	// BehaviorList indicates a collection retrieval (typically GET /resources).
	BehaviorList BehaviorType = "list"
	// BehaviorUpdate indicates a resource update (typically PUT/PATCH, or POST /{id} in some APIs).
	BehaviorUpdate BehaviorType = "update"
	// BehaviorDelete indicates a resource deletion (typically DELETE).
	BehaviorDelete BehaviorType = "delete"
	// BehaviorGeneric is the fallback for operations that don't map to CRUD (actions, webhooks, etc.).
	BehaviorGeneric BehaviorType = "generic"
)

// String returns the string representation of the behavior type.
func (bt BehaviorType) String() string {
	return string(bt)
}

// IsValid returns true if the behavior type is one of the defined constants.
func (bt BehaviorType) IsValid() bool {
	_, ok := validBehaviorTypes[bt]

	return ok
}

// ParseBehaviorType converts a string to a BehaviorType, returning
// ErrInvalidBehaviorType if the string is not recognized.
func ParseBehaviorType(s string) (BehaviorType, error) {
	bt := BehaviorType(s)
	if !bt.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidBehaviorType, s)
	}

	return bt, nil
}

// Validate checks that the BehaviorEntry has all required fields and valid values.
func (e *BehaviorEntry) Validate() error {
	if e.Method == "" {
		return ErrMissingMethod
	}

	if e.PathPattern == "" {
		return ErrMissingPathPattern
	}

	if !e.Type.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidBehaviorType, e.Type)
	}

	if e.SuccessCode == 0 {
		return ErrMissingSuccessCode
	}

	if !e.Source.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidSource, e.Source)
	}

	if e.Confidence < 0 || e.Confidence > 1 {
		return fmt.Errorf("%w: got %f", ErrInvalidConfidence, e.Confidence)
	}

	return nil
}

// BehaviorMapKey returns the map key for a given HTTP method and path pattern.
// This is the canonical way to construct lookup keys for BehaviorMap.
func BehaviorMapKey(method, pathPattern string) string {
	return method + " " + pathPattern
}

// BehaviorMap is an indexed collection of BehaviorEntry values, keyed by
// HTTP method + path pattern. It is the primary output of the behavior
// classifier and the input to the scenario router.
type BehaviorMap struct {
	entries map[string]BehaviorEntry
}

// NewBehaviorMap creates an empty BehaviorMap.
func NewBehaviorMap() *BehaviorMap {
	return &BehaviorMap{
		entries: make(map[string]BehaviorEntry),
	}
}

// Put adds or replaces a BehaviorEntry. The key is derived from the entry's
// Method and PathPattern fields.
func (bm *BehaviorMap) Put(entry BehaviorEntry) {
	key := BehaviorMapKey(entry.Method, entry.PathPattern)
	bm.entries[key] = entry
}

// Get retrieves a BehaviorEntry by HTTP method and path pattern.
// Returns the entry and true if found, or a zero value and false if not.
func (bm *BehaviorMap) Get(method, pathPattern string) (BehaviorEntry, bool) {
	key := BehaviorMapKey(method, pathPattern)
	entry, ok := bm.entries[key]

	return entry, ok
}

// Len returns the number of entries in the map.
func (bm *BehaviorMap) Len() int {
	return len(bm.entries)
}

// Entries returns all entries as a slice. Order is not guaranteed.
func (bm *BehaviorMap) Entries() []BehaviorEntry {
	result := make([]BehaviorEntry, 0, len(bm.entries))
	for _, e := range bm.entries {
		result = append(result, e)
	}

	return result
}
