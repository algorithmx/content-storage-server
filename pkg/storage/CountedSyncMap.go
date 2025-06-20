package storage

import (
	"sync"
	"sync/atomic"
)

// CountedSyncMap is a thread-safe map with an atomic counter for efficient size tracking
// It encapsulates sync.Map operations while maintaining an accurate count
type CountedSyncMap struct {
	items sync.Map // The underlying sync.Map for storing items
	count int64    // Atomic counter for the number of items
}

// NewCountedSyncMap creates a new CountedSyncMap
func NewCountedSyncMap() *CountedSyncMap {
	return &CountedSyncMap{
		count: 0,
	}
}

// Store sets the value for a key, returning whether this was a new key
func (csm *CountedSyncMap) Store(key, value interface{}) bool {
	_, wasPresent := csm.items.LoadOrStore(key, value)
	if wasPresent {
		// Update existing entry with new value
		csm.items.Store(key, value) // This IS necessary to update the value!
		return false                // Not a new key
	}
	// New entry - increment counter
	atomic.AddInt64(&csm.count, 1)
	return true // New key
}

// Load returns the value stored in the map for a key, or nil if no value is present
func (csm *CountedSyncMap) Load(key string) (value interface{}, ok bool) {
	return csm.items.Load(key)
}

// LoadOrStore returns the existing value for the key if present, otherwise stores and returns the given value
// The loaded result is true if the value was loaded, false if stored
func (csm *CountedSyncMap) LoadOrStore(key string, value interface{}) (actual interface{}, loaded bool) {
	actual, loaded = csm.items.LoadOrStore(key, value)
	if !loaded {
		// New entry was stored - increment counter
		atomic.AddInt64(&csm.count, 1)
	}
	return actual, loaded
}

// LoadAndDelete deletes the value for a key, returning the previous value if any
func (csm *CountedSyncMap) LoadAndDelete(key string) (value interface{}, loaded bool) {
	value, loaded = csm.items.LoadAndDelete(key)
	if loaded {
		// Item was present and deleted - decrement counter
		atomic.AddInt64(&csm.count, -1)
	}
	return value, loaded
}

// Delete deletes the value for a key
func (csm *CountedSyncMap) Delete(key string) {
	_, wasPresent := csm.items.LoadAndDelete(key)
	if wasPresent {
		atomic.AddInt64(&csm.count, -1)
	}
}

// Range calls f sequentially for each key and value present in the map
func (csm *CountedSyncMap) Range(f func(key string, value interface{}) bool) {
	csm.items.Range(func(key, value any) bool {
		return f(key.(string), value)
	})
}

// Count returns the current number of items in the map (O(1) operation)
func (csm *CountedSyncMap) Count() int {
	return int(atomic.LoadInt64(&csm.count))
}

// IsEmpty returns true if the map is empty (O(1) operation)
// This is more efficient than Count() == 0 for boolean checks
func (csm *CountedSyncMap) IsEmpty() bool {
	return atomic.LoadInt64(&csm.count) == 0
}
