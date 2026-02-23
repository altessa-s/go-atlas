// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import (
	"runtime"
	"sync"
	"weak"
)

// WeakMap is a concurrency-safe map that holds [weak.Pointer] references to its
// values. Entries are automatically removed when the garbage collector reclaims the
// referenced value, using [runtime.AddCleanup] to trigger deletion without periodic
// sweeps. This makes WeakMap suitable for caches where entries should not prevent
// garbage collection of the values they reference.
//
// All methods are safe for concurrent use by multiple goroutines.
//
// Because values may be collected at any time, [WeakMap.Len] returns an approximate
// count and [WeakMap.Get] may return (nil, false) even for a key that was recently
// set.
type WeakMap[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]weak.Pointer[V]
}

// NewWeakMap creates an empty [WeakMap] ready for use.
func NewWeakMap[K comparable, V any]() *WeakMap[K, V] {
	return &WeakMap[K, V]{
		data: make(map[K]weak.Pointer[V]),
	}
}

// Get retrieves the value associated with key. It returns the pointer and true if
// the key exists and the referenced value has not yet been garbage collected.
// If the key is missing or its value has been collected, Get returns (nil, false).
// The cleanup callback will eventually remove stale keys, so no manual action is needed.
func (m *WeakMap[K, V]) Get(key K) (*V, bool) {
	m.mu.RLock()
	wp, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		return nil, false
	}

	val := wp.Value()
	if val == nil {
		// Value was collected; the cleanup callback will remove this key.
		return nil, false
	}

	return val, true
}

// cleanupEntry holds the data needed to remove a map entry when a value is collected.
type cleanupEntry[K comparable, V any] struct {
	key K
	m   *WeakMap[K, V]
}

// Set associates key with val using a weak reference. The map does not prevent val
// from being garbage collected. When the GC reclaims val, the entry for key is
// automatically removed via [runtime.AddCleanup]. If val is nil, Set behaves like
// [WeakMap.Delete] and removes any existing entry for key.
func (m *WeakMap[K, V]) Set(key K, val *V) {
	if val == nil {
		m.Delete(key)
		return
	}

	wp := weak.Make(val)

	m.mu.Lock()
	m.data[key] = wp
	m.mu.Unlock()

	runtime.AddCleanup(val, func(e cleanupEntry[K, V]) {
		e.m.mu.Lock()
		// Only delete if the entry still points to the same (now-collected) weak pointer.
		// A new value may have been Set for this key in the meantime.
		if current, ok := e.m.data[e.key]; ok && current == wp {
			delete(e.m.data, e.key)
		}
		e.m.mu.Unlock()
	}, cleanupEntry[K, V]{key: key, m: m})
}

// Delete removes the entry for key from the map. If the key does not exist, Delete
// is a no-op. Any pending cleanup callback for the deleted key's value will detect
// the removal and skip its own delete.
func (m *WeakMap[K, V]) Delete(key K) {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
}

// Range calls f for each live (non-collected) key-value pair in the map. If f returns
// false, iteration stops early. Entries whose values have been garbage collected are
// removed from the map during the scan.
//
// Range takes a snapshot of all live entries under a write lock, then iterates the
// snapshot without holding any lock, so f may safely call other [WeakMap] methods.
func (m *WeakMap[K, V]) Range(f func(key K, value *V) bool) {
	type entry struct {
		key K
		val *V
	}

	m.mu.Lock()
	entries := make([]entry, 0, len(m.data))
	for k, wp := range m.data {
		val := wp.Value()
		if val == nil {
			delete(m.data, k)
			continue
		}
		entries = append(entries, entry{key: k, val: val})
	}
	m.mu.Unlock()

	for _, e := range entries {
		if !f(e.key, e.val) {
			return
		}
	}
}

// Len returns the approximate number of entries in the map. The count may include
// stale entries whose values have been garbage collected but whose cleanup callbacks
// have not yet executed. Call [WeakMap.Cleanup] first if an exact count is needed.
func (m *WeakMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Cleanup performs an explicit sweep of the map, removing all entries whose values
// have been garbage collected. This is normally unnecessary because [runtime.AddCleanup]
// handles removal automatically, but it can be useful before a call to [WeakMap.Len]
// when an accurate count is required.
func (m *WeakMap[K, V]) Cleanup() {
	m.mu.Lock()
	m.cleanupLocked()
	m.mu.Unlock()
}

func (m *WeakMap[K, V]) cleanupLocked() {
	for k, wp := range m.data {
		if wp.Value() == nil {
			delete(m.data, k)
		}
	}
}
