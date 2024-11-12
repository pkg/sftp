package sync

import (
	"sync"
)

// Map is a type-safe generic wrapper around sync.Map.
type Map[K comparable, V any] struct {
	sync.Map
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
// The value type parameter must be of a comparable type.
//
// If there is no current value for key in the map, CompareAndDelete returns false.
func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.Map.CompareAndDelete(key, old)
}

// CompareAndSwap swaps the old and new values for key if the value stored in the map is equal to old.
// The value type parameter must be of a comparable type.
func (m *Map[K, V]) CompareAndSwap(key K, old, new V) (swapped bool) {
	return m.Map.CompareAndSwap(key, old, new)
}

// Delete deletes the value for a key.
func (m *Map[K, V]) Delete(key K) {
	m.Map.Delete(key)
}

// Load returns the value stored in the map for a key,
// or the zero value if no value is present.
// The ok result indicates whether value was found in the map.
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.Map.Load(key)
	return v.(V), ok
}

// LoadAndDelete deletes the value for a key,
// returning the previous value if any.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, loaded := m.Map.LoadAndDelete(key)
	return v.(V), loaded
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, loaded := m.Map.LoadOrStore(key, value)
	return v.(V), loaded
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// The caveats noted in the standard library [sync.Map.Range] apply here as well.
func (m *Map[K, V]) Range(yield func(key K, value V) bool) {
	for k, v := range m.Map.Range {
		if !yield(k.(K), v.(V)) {
			return
		}
	}
}

// Store sets the value for a key.
func (m *Map[K, V]) Store(key K, value V) {
	m.Map.Store(key, value)
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	v, loaded := m.Map.Swap(key, value)
	return v.(V), loaded
}
