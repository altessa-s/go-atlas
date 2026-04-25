// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"fmt"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

// Default bucket configurations for Prometheus histogram metrics.
// These are shared between HTTP and gRPC transports.
var (
	// DefaultDurationBuckets provides reasonable latency buckets for requests.
	// Covers microsecond to multi-second request times.
	// Suitable for both HTTP and gRPC request duration tracking.
	DefaultDurationBuckets = []float64{
		0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
	}

	// DefaultSizeBuckets provides reasonable message size buckets for requests.
	// Covers very small messages (64 bytes) to large payloads (256MB).
	// Optimized to capture the full distribution of message sizes.
	DefaultSizeBuckets = []float64{
		64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456,
	}
)

// DefaultNamePartsCapacity is the default capacity for metric name parts slice.
// Used when building metric names with namespace and subsystem prefixes.
const DefaultNamePartsCapacity = 3

// ValidateBuckets checks that buckets is non-empty and in strictly
// increasing order, as required by Prometheus histogram registration.
// name is included in the error message to identify which bucket set
// failed validation.
func ValidateBuckets(buckets []float64, name string) error {
	if len(buckets) == 0 {
		return fmt.Errorf("%s buckets cannot be empty", name)
	}

	if !slices.IsStrictlyIncreasing(buckets) {
		return fmt.Errorf("%s buckets must be in strictly increasing order: %v", name, buckets)
	}

	return nil
}

// MustValidateBuckets is like [ValidateBuckets] but panics on failure.
// Intended for package-level init or var blocks where invalid buckets
// represent a programming error.
func MustValidateBuckets(buckets []float64, name string) {
	if err := ValidateBuckets(buckets, name); err != nil {
		panic(err)
	}
}

// CopyBuckets returns a shallow copy of buckets, preventing callers from
// mutating the shared default bucket slices.
func CopyBuckets(buckets []float64) []float64 {
	if buckets == nil {
		return nil
	}
	result := make([]float64, len(buckets))
	copy(result, buckets)
	return result
}
