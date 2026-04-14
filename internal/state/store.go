// Package state manages virtual resource state for stateful mock mode.
// In stateful mode, POST requests create resources, GET retrieves them,
// and DELETE removes them. The Store interface defines the contract for
// resource persistence, and InMemory provides the default implementation.
package state

// Store defines the contract for virtual resource storage.
// Resources are keyed by (namespace, scope, id) where namespace is derived
// from the URL path pattern (see [ResourceType]), scope isolates resources
// by parent path parameter values (see [ParentScope]), and id is extracted
// from the response body or path parameters.
type Store interface {
	// Get retrieves a stored resource. Returns the resource and true if found,
	// or nil and false if not.
	Get(namespace, scope, id string) (any, bool)

	// Put stores a resource, creating or replacing any existing resource with the
	// same namespace, scope, and id. Returns an error if the store cannot accept
	// the resource.
	Put(namespace, scope, id string, data any) error

	// Delete removes a resource. Returns true if the resource existed, false if not.
	Delete(namespace, scope, id string) bool

	// List returns all stored resources matching the given namespace and scope.
	// Returns an empty slice (not nil) if no matching resources exist.
	List(namespace, scope string) []any

	// Count returns the total number of stored resources across all namespaces
	// and scopes.
	Count() int

	// Reset removes all stored resources.
	Reset()
}
