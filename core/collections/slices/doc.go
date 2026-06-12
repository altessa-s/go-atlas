// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package slices provides generic utilities for slice operations like deduplication, filtering, and transformation.
// Complements the standard library with optimized operations for common patterns.
//
// All functions are read-only and safe for concurrent use. Functions create new slices/maps without modifying inputs.
//
// # Iterators
//
// Filter, Map, Chunk, and other collection operations return iter.Seq iterators for memory efficiency.
// Use slices.Collect() to materialize results into slices when needed.
//
// # Conventions
//
// Empty input handling: All functions return nil for empty input slices.
// This follows Go convention and allows easy nil checks.
//
// Slice to map conversion: For converting slices to maps, use
// [maps.FromSlice] and [maps.FromSliceWith] from the maps package.
//
// # Usage
//
//	nums := []int{1, 2, 2, 3, 1, 4}
//
//	// Deduplicate
//	unique := slices.Deduplicate(nums) // []int{1, 2, 3, 4}
//
//	// Filter (returns iter.Seq)
//	for n := range slices.Filter(nums, func(n int) bool { return n%2 == 0 }) {
//	    fmt.Println(n) // 2
//	}
//
//	// Advanced operations: Chunk returns iter.Seq[[]T]
//	for chunk := range slices.Chunk(nums, 3) {
//	    fmt.Println(chunk)
//	}
//
//	// Single-pass filter and transform
//	evenStrs := slices.ToWithFilter(nums,
//	    func(n int) bool { return n%2 == 0 },
//	    func(n int) string { return fmt.Sprintf("even:%d", n) },
//	)
//
// # Performance
//
//   - Small slice optimization: Slices ≤32 elements use linear scan for deduplication (25x faster than map-based).
//   - Parallel processing: MapParallel uses goroutines for large datasets (2x speedup for 50K+ elements).
//   - Zero allocations: FilterFirst, Any, All, Reduce have zero allocations with early return.
//   - Capacity optimization: Pre-allocates with optimal capacity to reduce reallocations.
package slices
