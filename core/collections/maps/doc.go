// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package maps provides utilities for map transformation, filtering, and conversion.
// Complements the standard library with merging, swapping, and flat map operations.
//
// All exported functions are pure and thread-safe.
//
// # Conventions
//
// Empty input handling: All functions return nil for empty input maps.
// This follows Go convention and allows easy nil checks.
//
// Slice to map conversion: Use [FromSlice] when you need the slice element
// as the map value, and [FromSliceWith] when you need custom key-value extraction.
//
// # Iterators (Go 1.23+)
//
// Keys, Values, Filter, Map return iter.Seq iterators for memory efficiency.
// Use slices.Collect() to materialize results into slices when needed.
//
// # Usage
//
//	m := map[string]int{"a": 1, "b": 2, "c": 3}
//
//	// Filter entries (returns map)
//	filtered := maps.FilterMap(m, func(k string, v int) bool {
//	    return v > 1
//	}) // map[string]int{"b": 2, "c": 3}
//
//	// Iterator-based operations
//	for k := range maps.Keys(m) {
//	    fmt.Println(k) // "a", "b", "c"
//	}
//
//	// Collect to slice when needed
//	values := stdslices.Collect(maps.Values(m)) // []int{1, 2, 3}
//
//	// Merge maps (src overwrites dst)
//	m1 := map[string]int{"a": 1, "b": 2}
//	m2 := map[string]int{"b": 3, "c": 4}
//	merged := maps.Merge(m2, m1) // map[string]int{"a": 1, "b": 3, "c": 4}
//
//	// Flat map operations
//	flat := map[string]any{"user.name": "Alice", "user.age": 25}
//	nested := maps.FromFlatMap(flat)
//	// map[user:map[age:25 name:Alice]]
//
// # Performance
//
// No notable overhead beyond standard library operations. Functions create new maps without modifying inputs.
package maps
