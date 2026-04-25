// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"fmt"
	"slices"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// Default bucket configurations for histogram metrics.
// These are shared across all adapters and components.
var (
	// DefaultDurationBuckets provides reasonable latency buckets for requests.
	// Covers microsecond to multi-second request times.
	// Suitable for HTTP, gRPC, and other request duration tracking.
	// Values are in seconds.
	DefaultDurationBuckets = []float64{
		0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
	}

	// DefaultSizeBuckets provides reasonable message size buckets.
	// Covers very small messages (64 bytes) to large payloads (256MB).
	// Values are in bytes.
	DefaultSizeBuckets = []float64{
		64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456,
	}

	// DefaultQuantileBuckets provides buckets for general quantile distribution.
	// Useful for tracking percentiles of any numeric value.
	DefaultQuantileBuckets = []float64{
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.95, 0.975, 0.99, 0.995,
	}
)

// DefaultNamePartsCapacity is the default capacity for metric name parts slice.
// Used when building metric names with namespace and subsystem prefixes.
const DefaultNamePartsCapacity = 3

// ValidateBuckets validates that histogram buckets are in strictly increasing order.
// Returns an error if validation fails.
func ValidateBuckets(buckets []float64, name string) error {
	if len(buckets) == 0 {
		return fmt.Errorf("%s buckets cannot be empty", name)
	}

	if !coreslices.IsStrictlyIncreasing(buckets) {
		return fmt.Errorf("%s buckets must be in strictly increasing order: %v", name, buckets)
	}

	return nil
}

// MustValidateBuckets validates that histogram buckets are in strictly increasing order.
// Panics if validation fails. Use for compile-time configuration validation.
func MustValidateBuckets(buckets []float64, name string) {
	if err := ValidateBuckets(buckets, name); err != nil {
		panic(err)
	}
}

// CopyBuckets creates a copy of the buckets slice.
// Used to prevent mutation of shared bucket configurations.
// Использует slices.Clone из стандартной библиотеки Go 1.21+.
func CopyBuckets(buckets []float64) []float64 {
	return slices.Clone(buckets)
}

// LinearBuckets creates count buckets of width width, starting at start.
// Example: LinearBuckets(0, 10, 5) creates [0, 10, 20, 30, 40].
func LinearBuckets(start, width float64, count int) []float64 {
	if count <= 0 {
		return nil
	}
	buckets := make([]float64, count)
	for i := range buckets {
		buckets[i] = start + float64(i)*width
	}
	return buckets
}

// ExponentialBuckets creates count buckets where the lowest bucket has an upper
// bound of start and each following bucket's upper bound is factor times the
// previous bucket's upper bound.
// Example: ExponentialBuckets(1, 2, 5) creates [1, 2, 4, 8, 16].
func ExponentialBuckets(start, factor float64, count int) []float64 {
	if count <= 0 || start <= 0 || factor <= 1 {
		return nil
	}
	buckets := make([]float64, count)
	buckets[0] = start
	for i := 1; i < count; i++ {
		buckets[i] = buckets[i-1] * factor
	}
	return buckets
}

// MergeBuckets merges multiple bucket slices into a single sorted slice
// with duplicates removed.
// Использует slices.Sort и slices.Compact из стандартной библиотеки Go 1.21+.
func MergeBuckets(bucketSets ...[]float64) []float64 {
	total := 0
	for _, buckets := range bucketSets {
		total += len(buckets)
	}
	if total == 0 {
		return nil
	}

	merged := make([]float64, 0, total)
	for _, buckets := range bucketSets {
		merged = append(merged, buckets...)
	}

	// Sort and remove duplicates using standard library
	slices.Sort(merged)
	return slices.Compact(merged)
}
