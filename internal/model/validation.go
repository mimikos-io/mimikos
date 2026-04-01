package model

// ValidationError represents a single request validation failure.
// Produced by the RequestValidator and consumed by the error Responder.
type ValidationError struct {
	// Field identifies what failed validation. The format depends on the
	// error source:
	//   - Request body fields use JSON pointer notation: "/name", "/address/city"
	//   - Parameter fields use dot notation: "query.limit", "path.userId"
	//   - Empty string for request-level errors (e.g., missing body entirely)
	Field string

	// Message describes the validation failure.
	Message string
}
