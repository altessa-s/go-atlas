// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "testing"

func BenchmarkLinearBuckets(b *testing.B) {
	for b.Loop() {
		LinearBuckets(0, 10, 20)
	}
}

func BenchmarkExponentialBuckets(b *testing.B) {
	for b.Loop() {
		ExponentialBuckets(1, 2, 20)
	}
}

func BenchmarkMergeBuckets(b *testing.B) {
	a := LinearBuckets(0, 10, 10)
	c := ExponentialBuckets(1, 2, 10)
	b.ResetTimer()
	for b.Loop() {
		MergeBuckets(a, c)
	}
}

func BenchmarkValidateBuckets(b *testing.B) {
	buckets := DefaultDurationBuckets
	b.ResetTimer()
	for b.Loop() {
		_ = ValidateBuckets(buckets, "bench")
	}
}
