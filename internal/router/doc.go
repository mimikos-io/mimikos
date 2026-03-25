// Package router implements the HTTP router and scenario router.
// The HTTP router matches incoming requests to OpenAPI operations.
// The scenario router selects the appropriate response scenario
// (success, validation error, not found) based on the behavior map.
package router
