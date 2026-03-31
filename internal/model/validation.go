package model

// ValidationError represents a single request validation failure.
// Produced by the RequestValidator and consumed by the error Responder.
type ValidationError struct {
	// Field is the JSON pointer or parameter name that failed validation
	// (e.g., "/body/name", "query.limit"). Empty string for request-level
	// errors such as a missing body entirely.
	Field string

	// Message describes the validation failure.
	Message string
}
