// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import "testing"

func BenchmarkValidateBuckets(b *testing.B) {
	buckets := DefaultDurationBuckets
	for b.Loop() {
		ValidateBuckets(buckets, "test")
	}
}

func BenchmarkCopyBuckets(b *testing.B) {
	buckets := DefaultDurationBuckets
	for b.Loop() {
		CopyBuckets(buckets)
	}
}

func BenchmarkBuildMetricName(b *testing.B) {
	var s string
	for b.Loop() {
		s = BuildMetricName("myapp", "http", "requests_total")
	}
	_ = s
}

func BenchmarkGetMessageSize(b *testing.B) {
	msg := &benchSizer{size: 42}
	for b.Loop() {
		GetMessageSize(msg)
	}
}

type benchSizer struct{ size int }

func (m *benchSizer) Size() int { return m.size }
