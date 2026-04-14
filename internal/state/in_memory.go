package state

import (
	"container/list"
	"sort"
	"sync"
)

// entry wraps stored resource data with LRU tracking metadata.
type entry struct {
	namespace string
	scope     string
	id        string
	data      any
	element   *list.Element
}

// InMemory is an in-memory Store implementation with LRU eviction.
// It is safe for concurrent use. All state is lost on server restart.
// Resources are stored in a three-level map: namespace → scope → id.
type InMemory struct {
	mu        sync.Mutex
	resources map[string]map[string]map[string]*entry // [namespace][scope][id]
	lru       *list.List
	capacity  int
}

// NewInMemory creates a new in-memory store. Capacity sets the maximum number
// of resources across all types. A capacity of 0 or less means unlimited.
func NewInMemory(capacity int) *InMemory {
	if capacity < 0 {
		capacity = 0
	}

	return &InMemory{
		resources: make(map[string]map[string]map[string]*entry),
		lru:       list.New(),
		capacity:  capacity,
	}
}

// Get retrieves a stored resource and refreshes its LRU position.
func (m *InMemory) Get(namespace, scope, id string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	scopeEntries, ok := m.resources[namespace]
	if !ok {
		return nil, false
	}

	idEntries, ok := scopeEntries[scope]
	if !ok {
		return nil, false
	}

	entry, ok := idEntries[id]
	if !ok {
		return nil, false
	}

	m.lru.MoveToFront(entry.element)

	return entry.data, true
}

// Put stores a resource, creating or replacing any existing resource with the
// same namespace, scope, and id. If the store is at capacity, the least
// recently used resource is evicted.
func (m *InMemory) Put(namespace, scope, id string, data any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update existing entry (fast path).
	if scopeEntries, ok := m.resources[namespace]; ok {
		if idEntries, ok := scopeEntries[scope]; ok {
			if entry, exists := idEntries[id]; exists {
				entry.data = data
				m.lru.MoveToFront(entry.element)

				return nil
			}
		}
	}

	// Evict if at capacity. Must happen before we resolve maps — eviction
	// may delete intermediate maps, invalidating any earlier local reference.
	if m.capacity > 0 && m.lru.Len() >= m.capacity {
		m.evict()
	}

	scopeEntries, ok := m.resources[namespace]
	if !ok {
		scopeEntries = make(map[string]map[string]*entry)
		m.resources[namespace] = scopeEntries
	}

	idEntries, ok := scopeEntries[scope]
	if !ok {
		idEntries = make(map[string]*entry)
		scopeEntries[scope] = idEntries
	}

	e := &entry{
		namespace: namespace,
		scope:     scope,
		id:        id,
		data:      data,
	}
	e.element = m.lru.PushFront(e)
	idEntries[id] = e

	return nil
}

// Delete removes a resource. Returns true if the resource existed.
func (m *InMemory) Delete(namespace, scope, id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	scopeEntries, ok := m.resources[namespace]
	if !ok {
		return false
	}

	idEntries, ok := scopeEntries[scope]
	if !ok {
		return false
	}

	entry, ok := idEntries[id]
	if !ok {
		return false
	}

	m.lru.Remove(entry.element)
	delete(idEntries, id)

	// Clean up empty intermediate maps.
	if len(idEntries) == 0 {
		delete(scopeEntries, scope)
	}

	if len(scopeEntries) == 0 {
		delete(m.resources, namespace)
	}

	return true
}

// List returns all stored resources matching the given namespace and scope,
// sorted by resource ID for deterministic ordering. Returns an empty slice
// (not nil) if no matching resources exist.
func (m *InMemory) List(namespace, scope string) []any {
	m.mu.Lock()
	defer m.mu.Unlock()

	scopeEntries := m.resources[namespace]
	if len(scopeEntries) == 0 {
		return make([]any, 0)
	}

	idEntries := scopeEntries[scope]
	if len(idEntries) == 0 {
		return make([]any, 0)
	}

	// Collect entries and sort by ID for deterministic iteration order.
	sorted := make([]*entry, 0, len(idEntries))
	for _, e := range idEntries {
		sorted = append(sorted, e)
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].id < sorted[j].id
	})

	result := make([]any, len(sorted))
	for i, e := range sorted {
		result[i] = e.data
	}

	return result
}

// Count returns the total number of stored resources across all types.
func (m *InMemory) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.lru.Len()
}

// Reset removes all stored resources.
func (m *InMemory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.resources = make(map[string]map[string]map[string]*entry)
	m.lru.Init()
}

// evict removes the least recently used entry. Caller must hold m.mu.
func (m *InMemory) evict() {
	back := m.lru.Back()
	if back == nil {
		return
	}

	e, _ := back.Value.(*entry)
	m.lru.Remove(back)

	scopeEntries := m.resources[e.namespace]
	idEntries := scopeEntries[e.scope]
	delete(idEntries, e.id)

	// Clean up empty intermediate maps to avoid accumulating empties.
	if len(idEntries) == 0 {
		delete(scopeEntries, e.scope)
	}

	if len(scopeEntries) == 0 {
		delete(m.resources, e.namespace)
	}
}
