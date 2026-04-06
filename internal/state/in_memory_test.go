package state

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CRUD tests ---

func TestInMemory_PutGet_RoundTrip(t *testing.T) {
	s := NewInMemory(0)
	data := map[string]any{"name": "Fido"}

	require.NoError(t, s.Put("pets", "1", data))

	got, ok := s.Get("pets", "1")
	require.True(t, ok)
	assert.Equal(t, data, got)
}

func TestInMemory_Get_NonExistent(t *testing.T) {
	s := NewInMemory(0)

	got, ok := s.Get("pets", "999")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestInMemory_Get_WrongType(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))

	got, ok := s.Get("users", "1")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestInMemory_Delete_Existing(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))

	assert.True(t, s.Delete("pets", "1"))
}

func TestInMemory_Delete_NonExistent(t *testing.T) {
	s := NewInMemory(0)
	assert.False(t, s.Delete("pets", "999"))
}

func TestInMemory_Delete_ThenGet(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	s.Delete("pets", "1")

	got, ok := s.Get("pets", "1")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestInMemory_Put_Overwrite(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	require.NoError(t, s.Put("pets", "1", "Rex"))

	got, ok := s.Get("pets", "1")
	require.True(t, ok)
	assert.Equal(t, "Rex", got)
	assert.Equal(t, 1, s.Count(), "overwrite should not increase count")
}

func TestInMemory_List(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	require.NoError(t, s.Put("pets", "2", "Rex"))
	require.NoError(t, s.Put("users", "1", "Alice"))

	pets := s.List("pets")
	assert.Len(t, pets, 2)
	assert.Contains(t, pets, any("Fido"))
	assert.Contains(t, pets, any("Rex"))
}

func TestInMemory_List_DeterministicOrder(t *testing.T) {
	s := NewInMemory(0)

	// Insert in non-alphabetical order.
	require.NoError(t, s.Put("pets", "c", "Charlie"))
	require.NoError(t, s.Put("pets", "a", "Alpha"))
	require.NoError(t, s.Put("pets", "b", "Bravo"))

	// List should always return sorted by ID.
	for range 10 {
		pets := s.List("pets")
		require.Len(t, pets, 3)
		assert.Equal(t, "Alpha", pets[0])
		assert.Equal(t, "Bravo", pets[1])
		assert.Equal(t, "Charlie", pets[2])
	}
}

func TestInMemory_List_EmptyType(t *testing.T) {
	s := NewInMemory(0)

	result := s.List("pets")
	assert.NotNil(t, result, "empty list should be non-nil")
	assert.Empty(t, result)
}

func TestInMemory_List_AfterDelete(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	require.NoError(t, s.Put("pets", "2", "Rex"))
	s.Delete("pets", "1")

	pets := s.List("pets")
	assert.Len(t, pets, 1)
	assert.Contains(t, pets, any("Rex"))
}

func TestInMemory_Count(t *testing.T) {
	s := NewInMemory(0)
	assert.Equal(t, 0, s.Count())

	require.NoError(t, s.Put("pets", "1", "Fido"))
	assert.Equal(t, 1, s.Count())

	require.NoError(t, s.Put("users", "1", "Alice"))
	assert.Equal(t, 2, s.Count())

	require.NoError(t, s.Put("pets", "2", "Rex"))
	assert.Equal(t, 3, s.Count())
}

func TestInMemory_Reset(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	require.NoError(t, s.Put("users", "1", "Alice"))

	s.Reset()

	assert.Equal(t, 0, s.Count())

	_, ok := s.Get("pets", "1")
	assert.False(t, ok)

	assert.Empty(t, s.List("pets"))
	assert.Empty(t, s.List("users"))
}

// --- Concurrency tests ---

func TestInMemory_ConcurrentPut(t *testing.T) {
	s := NewInMemory(0)

	var wg sync.WaitGroup

	for g := range 10 {
		wg.Add(1)

		go func(gID int) {
			defer wg.Done()

			for i := range 100 {
				id := strconv.Itoa(gID) + "-" + strconv.Itoa(i)
				_ = s.Put("items", id, id)
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, 1000, s.Count())
}

func TestInMemory_ConcurrentGetDuringPut(t *testing.T) {
	s := NewInMemory(0)
	require.NoError(t, s.Put("pets", "seed", "initial"))

	var wg sync.WaitGroup

	// Writers.
	for g := range 5 {
		wg.Add(1)

		go func(gID int) {
			defer wg.Done()

			for i := range 100 {
				id := strconv.Itoa(gID) + "-" + strconv.Itoa(i)
				_ = s.Put("pets", id, id)
			}
		}(g)
	}

	// Readers.
	for range 5 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 100 {
				s.Get("pets", "seed")
				s.List("pets")
				s.Count()
			}
		}()
	}

	wg.Wait()
	// No panic, no data race — that's the assertion.
}

func TestInMemory_ConcurrentPutWithEviction(t *testing.T) {
	s := NewInMemory(50)

	var wg sync.WaitGroup

	for g := range 10 {
		wg.Add(1)

		go func(gID int) {
			defer wg.Done()

			for i := range 100 {
				id := strconv.Itoa(gID) + "-" + strconv.Itoa(i)
				_ = s.Put("items", id, id)
			}
		}(g)
	}

	wg.Wait()
	assert.LessOrEqual(t, s.Count(), 50)
}

func TestInMemory_ConcurrentDeleteAndList(t *testing.T) {
	s := NewInMemory(0)

	// Seed with 100 items.
	for i := range 100 {
		require.NoError(t, s.Put("items", strconv.Itoa(i), i))
	}

	var wg sync.WaitGroup

	// Deleters.
	for g := range 5 {
		wg.Add(1)

		go func(gID int) {
			defer wg.Done()

			for i := range 20 {
				s.Delete("items", strconv.Itoa(gID*20+i))
			}
		}(g)
	}

	// Listers.
	for range 5 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 50 {
				s.List("items")
				s.Count()
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 0, s.Count())
}

// --- LRU eviction tests ---

func TestInMemory_LRU_EvictsOldest(t *testing.T) {
	s := NewInMemory(3)
	require.NoError(t, s.Put("t", "a", "A"))
	require.NoError(t, s.Put("t", "b", "B"))
	require.NoError(t, s.Put("t", "c", "C"))

	// Capacity full. Adding "d" should evict "a" (oldest).
	require.NoError(t, s.Put("t", "d", "D"))

	_, ok := s.Get("t", "a")
	assert.False(t, ok, "oldest entry 'a' should be evicted")

	_, ok = s.Get("t", "d")
	assert.True(t, ok, "newest entry 'd' should exist")

	assert.Equal(t, 3, s.Count())
}

func TestInMemory_LRU_GetRefreshes(t *testing.T) {
	s := NewInMemory(3)
	require.NoError(t, s.Put("t", "a", "A"))
	require.NoError(t, s.Put("t", "b", "B"))
	require.NoError(t, s.Put("t", "c", "C"))

	// Touch "a" to refresh it.
	s.Get("t", "a")

	// Add "d" — should evict "b" (now the oldest untouched).
	require.NoError(t, s.Put("t", "d", "D"))

	_, ok := s.Get("t", "a")
	assert.True(t, ok, "'a' was refreshed and should survive eviction")

	_, ok = s.Get("t", "b")
	assert.False(t, ok, "'b' should be evicted as oldest untouched")
}

func TestInMemory_LRU_CrossType(t *testing.T) {
	s := NewInMemory(3)
	require.NoError(t, s.Put("pets", "1", "Fido"))
	require.NoError(t, s.Put("users", "1", "Alice"))
	require.NoError(t, s.Put("pets", "2", "Rex"))

	// Eviction is global, not per-type. Adding one more should evict pets/1.
	require.NoError(t, s.Put("users", "2", "Bob"))

	_, ok := s.Get("pets", "1")
	assert.False(t, ok, "oldest entry (pets/1) should be evicted regardless of type")

	assert.Equal(t, 3, s.Count())
}

func TestInMemory_LRU_UnlimitedCapacity(t *testing.T) {
	s := NewInMemory(0)

	for i := range 100 {
		require.NoError(t, s.Put("t", strconv.Itoa(i), i))
	}

	assert.Equal(t, 100, s.Count(), "capacity 0 means unlimited — no eviction")
}

func TestInMemory_LRU_PutExistingRefreshes(t *testing.T) {
	s := NewInMemory(3)
	require.NoError(t, s.Put("t", "a", "A"))
	require.NoError(t, s.Put("t", "b", "B"))
	require.NoError(t, s.Put("t", "c", "C"))

	// Overwrite "a" — should refresh its LRU position.
	require.NoError(t, s.Put("t", "a", "A2"))

	// Add "d" — should evict "b" (now oldest), not "a".
	require.NoError(t, s.Put("t", "d", "D"))

	got, ok := s.Get("t", "a")
	assert.True(t, ok, "'a' was refreshed by Put and should survive")
	assert.Equal(t, "A2", got, "data should be updated")

	_, ok = s.Get("t", "b")
	assert.False(t, ok, "'b' should be evicted as oldest")
}

func TestInMemory_LRU_CapacityOne(t *testing.T) {
	s := NewInMemory(1)
	require.NoError(t, s.Put("t", "a", "A"))

	// Adding "b" should evict "a".
	require.NoError(t, s.Put("t", "b", "B"))

	_, ok := s.Get("t", "a")
	assert.False(t, ok, "'a' should be evicted")

	got, ok := s.Get("t", "b")
	assert.True(t, ok)
	assert.Equal(t, "B", got)
	assert.Equal(t, 1, s.Count())
}

func TestInMemory_LRU_NegativeCapacity(t *testing.T) {
	s := NewInMemory(-5)

	for i := range 10 {
		require.NoError(t, s.Put("t", strconv.Itoa(i), i))
	}

	assert.Equal(t, 10, s.Count(), "negative capacity treated as unlimited")
}

// --- Interface compliance ---

func TestInMemory_ImplementsStore(_ *testing.T) {
	var _ Store = (*InMemory)(nil)
}
