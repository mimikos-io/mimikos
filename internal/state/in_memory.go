package state

import (
	"container/list"
	"sync"
)

// entry wraps stored resource data with LRU tracking metadata.
type entry struct {
	resourceType string
	id           string
	data         any
	element      *list.Element
}

// InMemory is an in-memory Store implementation with LRU eviction.
// It is safe for concurrent use. All state is lost on server restart.
type InMemory struct {
	mu        sync.Mutex
	resources map[string]map[string]*entry
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
		resources: make(map[string]map[string]*entry),
		lru:       list.New(),
		capacity:  capacity,
	}
}

// Get retrieves a stored resource and refreshes its LRU position.
func (m *InMemory) Get(resourceType, id string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resourceEntries, ok := m.resources[resourceType]
	if !ok {
		return nil, false
	}

	entry, ok := resourceEntries[id]
	if !ok {
		return nil, false
	}

	m.lru.MoveToFront(entry.element)

	return entry.data, true
}

// Put stores a resource, creating or replacing any existing resource with the
// same type and id. If the store is at capacity, the least recently used
// resource is evicted.
func (m *InMemory) Put(resourceType, id string, data any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update existing entry (fast path).
	if resourceEntries, ok := m.resources[resourceType]; ok {
		if entry, exists := resourceEntries[id]; exists {
			entry.data = data
			m.lru.MoveToFront(entry.element)

			return nil
		}
	}

	// Evict if at capacity. Must happen before we resolve resourceEntries — eviction
	// may delete a type's map, invalidating any earlier local reference.
	if m.capacity > 0 && m.lru.Len() >= m.capacity {
		m.evict()
	}

	resourceEntries, ok := m.resources[resourceType]
	if !ok {
		resourceEntries = make(map[string]*entry)
		m.resources[resourceType] = resourceEntries
	}

	e := &entry{
		resourceType: resourceType,
		id:           id,
		data:         data,
	}
	e.element = m.lru.PushFront(e)
	resourceEntries[id] = e

	return nil
}

// Delete removes a resource. Returns true if the resource existed.
func (m *InMemory) Delete(resourceType, id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	resourceEntries, ok := m.resources[resourceType]
	if !ok {
		return false
	}

	entry, ok := resourceEntries[id]
	if !ok {
		return false
	}

	m.lru.Remove(entry.element)
	delete(resourceEntries, id)

	if len(resourceEntries) == 0 {
		delete(m.resources, resourceType)
	}

	return true
}

// List returns all stored resources of the given type. Returns an empty
// slice (not nil) if no resources of that type exist.
func (m *InMemory) List(resourceType string) []any {
	m.mu.Lock()
	defer m.mu.Unlock()

	resourceEntries := m.resources[resourceType]
	result := make([]any, 0, len(resourceEntries))

	for _, entry := range resourceEntries {
		result = append(result, entry.data)
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

	m.resources = make(map[string]map[string]*entry)
	m.lru.Init()
}

// evict removes the least recently used entry. Caller must hold m.mu.
func (m *InMemory) evict() {
	back := m.lru.Back()
	if back == nil {
		return
	}

	entry, _ := back.Value.(*entry)
	m.lru.Remove(back)

	resourceEntries := m.resources[entry.resourceType]
	delete(resourceEntries, entry.id)

	// Remove the type's map when its last entry is evicted to avoid
	// accumulating empty maps in m.resources across eviction cycles.
	if len(resourceEntries) == 0 {
		delete(m.resources, entry.resourceType)
	}
}
