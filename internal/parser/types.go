package parser

import (
	"errors"

	"github.com/pb33f/libopenapi/datamodel/high/base"
)

type (
	// ParsedSpec is the normalized output of parsing an OpenAPI specification.
	// It contains all information needed by the Behavior Classifier (Task 7)
	// and Schema Compiler (Session 10).
	ParsedSpec struct {
		// Version is the OpenAPI version string (e.g., "3.0.0", "3.1.0").
		// Used by Schema Compiler for 3.0/3.1 normalization decisions.
		Version string

		// Title is the API title from info.title.
		Title string

		// Operations is the ordered list of all operations in the spec.
		// Order matches the spec's source order (deterministic).
		Operations []Operation

		// CircularRefs lists all circular references detected during parsing.
		// Used for startup diagnostics/logging. Individual schemas are also
		// annotated via SchemaRef.IsCircular.
		CircularRefs []CircularRef
	}

	// Operation represents a single API operation extracted from the spec.
	Operation struct {
		// Method is the uppercase HTTP method (GET, POST, PUT, PATCH, DELETE).
		Method string

		// Path is the OpenAPI path template (e.g., "/pets/{petId}").
		Path string

		// OperationID is the operationId from the spec. May be empty —
		// not all specs define it (auto-generated specs frequently omit it).
		OperationID string

		// Summary is the operation summary. Used for logging only.
		Summary string

		// Tags from the operation. May be empty.
		Tags []string

		// RequestBody is the request body schema, or nil if the operation
		// has no request body (e.g., GET, DELETE).
		RequestBody *RequestBody

		// Responses maps HTTP status codes to their response definitions.
		// Only includes responses that have JSON content (application/json).
		Responses map[int]*Response

		// DefaultResponse is the "default" response, if defined in the spec.
		// Separate from Responses because "default" has no integer status code.
		// Nil if not defined.
		DefaultResponse *Response
	}

	// RequestBody represents an operation's request body.
	RequestBody struct {
		// Required indicates whether the request body is required.
		Required bool

		// Schema is the JSON request body schema.
		// Extracted from the application/json content type.
		// Nil if no application/json content is defined.
		Schema *SchemaRef
	}

	// Response represents a single HTTP response definition.
	Response struct {
		// StatusCode is the HTTP status code (e.g., 200, 201, 404).
		// Zero for "default" responses.
		StatusCode int

		// Description is the response description from the spec.
		Description string

		// Schema is the JSON response body schema.
		// Extracted from the application/json content type.
		// Nil if the response has no JSON content (e.g., 204 No Content).
		Schema *SchemaRef
	}

	// SchemaRef is a resolved, annotated schema reference.
	// It wraps the libopenapi base.Schema with parser-added metadata.
	SchemaRef struct {
		// Name is a human-readable identifier for the schema.
		// For component schemas: the key from components/schemas (e.g., "Pet").
		// For inline schemas: generated from context (e.g., "listPets_200_response").
		Name string

		// IsCircular indicates this schema participates in a circular
		// reference chain. The Data Generator must apply max-depth cutoff.
		IsCircular bool

		// Raw is the resolved libopenapi schema. All $ref pointers have been
		// followed — this is the concrete schema, never a proxy.
		//
		// Exposed for the Schema Compiler (Session 10) which needs full schema
		// access to compile into jsonschema validators.
		Raw *base.Schema
	}

	// CircularRef records a circular reference detected during parsing.
	// Used for startup logging and diagnostics.
	CircularRef struct {
		// SchemaName is the starting schema in the cycle.
		SchemaName string

		// JourneyPath is the human-readable reference chain
		// (e.g., "Category -> parent -> Category").
		JourneyPath string

		// IsInfiniteLoop is true when all fields in the cycle are required,
		// making the cycle truly unresolvable (not just optional nesting).
		IsInfiniteLoop bool

		// IsPolymorphic is true when the cycle passes through oneOf/anyOf/allOf.
		IsPolymorphic bool

		// IsArray is true when the cycle passes through an array items reference.
		IsArray bool
	}
)

// Sentinel errors for parser failures.
var (
	// ErrEmptyInput is returned when the input data is empty or nil.
	ErrEmptyInput = errors.New("parser: empty input")

	// ErrInvalidSpec is returned when the input is not a valid OpenAPI spec.
	ErrInvalidSpec = errors.New("parser: invalid OpenAPI spec")

	// ErrUnsupportedVersion is returned for OpenAPI 2.0 (Swagger) or unrecognized versions.
	ErrUnsupportedVersion = errors.New("parser: unsupported OpenAPI version")

	// ErrBuildModel is returned when libopenapi fails to build the document model
	// and the resulting model is nil (unrecoverable).
	ErrBuildModel = errors.New("parser: failed to build document model")
)
