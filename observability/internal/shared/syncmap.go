// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"sync"
)

// GetOrCreate is a generic helper for retrieving or creating values in a sync.Map.
// It implements a double-check pattern for thread-safe lazy initialization.
//
// Example:
//
//	var tracers sync.Map
//	tracer := GetOrCreate(&tracers, "mytracer", func() *Tracer {
//	    return newTracer("mytracer")
//	})
func GetOrCreate[T any](m *sync.Map, key string, create func() T) T {
	// Fast path: value already exists
	if existing, ok := m.Load(key); ok {
		if t, ok := existing.(T); ok {
			return t
		}
	}

	// Create new value
	newVal := create()

	// Store or get existing (handles race condition)
	actual, _ := m.LoadOrStore(key, newVal)
	return actual.(T) //nolint:errcheck // Type is guaranteed by generic constraint
}

// GetOrCreateWithCallback is like GetOrCreate but calls an optional callback
// when a new value is created. Useful for registration or initialization side effects.
//
// Example:
//
//	counter := GetOrCreateWithCallback(&metrics, name, createCounter, func() {
//	    registerMetric(name)
//	})
func GetOrCreateWithCallback[T any](m *sync.Map, key string, create func() T, onNew func()) T {
	// Fast path: value already exists
	if existing, ok := m.Load(key); ok {
		if t, ok := existing.(T); ok {
			return t
		}
	}

	// Create new value
	newVal := create()

	// Store or get existing (handles race condition)
	actual, loaded := m.LoadOrStore(key, newVal)

	// Call callback only if we stored a new value
	if !loaded && onNew != nil {
		onNew()
	}

	return actual.(T) //nolint:errcheck // Type is guaranteed by generic constraint
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
