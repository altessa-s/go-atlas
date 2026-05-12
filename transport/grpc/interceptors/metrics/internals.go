// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"fmt"
	"slices"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// Metric label names used by the interceptor. Compile-time constants;
// the Go runtime points all references at the same backing string so no
// runtime interning is required for these short keys.
const (
	methodLabel    = "method"
	statusLabel    = "status"
	directionLabel = "direction"
)

// Direction label values for streaming metrics.
const (
	directionSent     = "sent"
	directionReceived = "received"
)

// Default bucket configurations for histogram metrics.
//
//nolint:gochecknoglobals // tunables exported for godoc and reuse.
var (
	// DefaultDurationBuckets provides reasonable latency buckets for gRPC
	// requests. Covers microsecond to multi-second request times.
	DefaultDurationBuckets = []float64{
		0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
	}

	// DefaultSizeBuckets provides reasonable message size buckets for
	// gRPC requests. Covers 64 bytes to 256 MB.
	DefaultSizeBuckets = []float64{
		64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456,
	}
)

// validateBuckets checks that buckets is non-empty and in strictly
// increasing order. name identifies which bucket set failed validation.
func validateBuckets(buckets []float64, name string) error {
	if len(buckets) == 0 {
		return fmt.Errorf("%s buckets cannot be empty", name)
	}
	if !coreslices.IsStrictlyIncreasing(buckets) {
		return fmt.Errorf("%s buckets must be in strictly increasing order: %v", name, buckets)
	}
	return nil
}

// mustValidateBuckets panics if buckets fails validation. Used by
// option setters where invalid buckets are a programming error.
func mustValidateBuckets(buckets []float64, name string) {
	if err := validateBuckets(buckets, name); err != nil {
		panic(err)
	}
}

// getMessageSize extracts the serialized byte size of msg by probing
// known sizing interfaces:
//
//  1. ProtoSize() int -- gogoproto-generated messages
//  2. Size() int      -- standard google.golang.org/protobuf or custom
//  3. XXX_Size() int  -- legacy proto2/proto3 generated code
//
// Returns (0, false) when msg is nil or implements none of these.
func getMessageSize(msg any) (int, bool) {
	if msg == nil {
		return 0, false
	}
	if protoMsg, ok := msg.(interface{ ProtoSize() int }); ok {
		return protoMsg.ProtoSize(), true
	}
	if sizeable, ok := msg.(interface{ Size() int }); ok {
		return sizeable.Size(), true
	}
	if legacyMsg, ok := msg.(interface{ XXX_Size() int }); ok {
		return legacyMsg.XXX_Size(), true
	}
	return 0, false
}

// cloneBuckets returns a defensive copy of buckets. Wraps stdlib
// [slices.Clone] for clarity at call sites.
func cloneBuckets(buckets []float64) []float64 { return slices.Clone(buckets) }
