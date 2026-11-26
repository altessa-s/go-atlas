// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"sync"
)

// GetOrCreate is a generic helper for retrieving or creating values in a sync.Map.
// It implements a double-check pattern for thread-safe lazy initialization.
// If an entry already exists under key with a different concrete type than T,
// the zero value of T is returned without overwriting the existing entry.
//
// Example:
//
//	var tracers sync.Map
//	tracer := GetOrCreate(&tracers, "mytracer", func() *Tracer {
//	    return newTracer("mytracer")
//	})
func GetOrCreate[T any, K comparable](m *sync.Map, key K, create func() T) T {
	// Fast path: value already exists
	if existing, ok := m.Load(key); ok {
		if t, ok := existing.(T); ok {
			return t
		}
		var zero T
		return zero
	}

	// Create new value
	newVal := create()

	// Store or get existing (handles race condition)
	actual, _ := m.LoadOrStore(key, newVal)
	if t, ok := actual.(T); ok {
		return t
	}
	var zero T
	return zero
}

// GetOrCreateWithCallback is like GetOrCreate but calls an optional callback
// when a new value is created. Useful for registration or initialization side effects.
//
// Creation and onNew run under mu before the value is published, so no
// caller can obtain the value until onNew has completed. Lookups of existing
// values stay lock-free. If an entry already exists under key with a different
// concrete type than T, the zero value of T is returned without running create
// or onNew and without overwriting the existing entry.
//
// Example:
//
//	counter := GetOrCreateWithCallback(&metrics, &mu, name, createCounter, func() {
//	    registerMetric(name)
//	})
func GetOrCreateWithCallback[T any](m *sync.Map, mu *sync.Mutex, key string, create func() T, onNew func()) T {
	// Fast path: value already exists
	if existing, ok := m.Load(key); ok {
		if t, ok := existing.(T); ok {
			return t
		}
		var zero T
		return zero
	}

	mu.Lock()
	defer mu.Unlock()

	// Re-check: another caller may have published it while we waited.
	if existing, ok := m.Load(key); ok {
		if t, ok := existing.(T); ok {
			return t
		}
		var zero T
		return zero
	}

	newVal := create()
	if onNew != nil {
		onNew()
	}
	m.Store(key, newVal)

	return newVal
}

// Range iterates over all key-value pairs in a sync.Map with type-safe values.
// The iteration stops when the callback returns false.
//
// Example:
//
//	Range(&tracers, func(name string, tracer *Tracer) bool {
//	    fmt.Println(name, tracer)
//	    return true // continue iteration
//	})
func Range[T any](m *sync.Map, fn func(key string, value T) bool) {
	m.Range(func(k, v any) bool {
		key, ok := k.(string)
		if !ok {
			return true // skip non-string keys
		}
		val, ok := v.(T)
		if !ok {
			return true // skip wrong type values
		}
		return fn(key, val)
	})
}
