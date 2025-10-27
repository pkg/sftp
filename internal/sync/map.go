package sync

import (
	"sync"
)

// Map is a type-safe generic wrapper around sync.m.
type Map[K comparable, V any] struct {
	m sync.Map
}

// Clear deletes all the entries, resulting in an empty Map.
func (m *Map[K, V]) Clear() {
	m.m.Clear()
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
// The value type parameter must be of a comparable type.
//
// If there is no current value for key in the map, CompareAndDelete returns false.
func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

// CompareAndSwap swaps the old and new values for key if the value stored in the map is equal to old.
// The value type parameter must be of a comparable type.
func (m *Map[K, V]) CompareAndSwap(key K, old, new V) (swapped bool) {
	return m.m.CompareAndSwap(key, old, new)
}

// Delete deletes the value for a key.
func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}

// Load returns the value stored in the map for a key,
// or the zero value if no value is present.
// The ok result indicates whether value was found in the map.
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	if v, ok := m.m.Load(key); ok {
		return v.(V), true
	}
	return value, false
}

// LoadAndDelete deletes the value for a key,
// returning the previous value if any.
// If there is no previous value, it returns the zero value.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	if v, loaded := m.m.LoadAndDelete(key); loaded {
		return v.(V), true
	}
	return value, false
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, loaded := m.m.LoadOrStore(key, value)
	return v.(V), loaded
}

// Range calls f sequentially for each key and value present in the map.
// If yield returns false, range stops the iteration.
//
// The caveats noted in the standard library [sync.m.Range] apply here as well.
func (m *Map[K, V]) Range(yield func(key K, value V) bool) {
	for k, v := range m.m.Range {
		if !yield(k.(K), v.(V)) {
			return
		}
	}
}

// Store sets the value for a key.
func (m *Map[K, V]) Store(key K, value V) {
	m.m.Store(key, value)
}

// Swap swaps the value for a key and returns the previous value if any.
// If there is no previous value, the previous result will be the zero value.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	if v, loaded := m.m.Swap(key, value); loaded {
		return v.(V), true
	}
	return previous, false
}
