// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import "testing"

func BenchmarkBuildMetricName(b *testing.B) {
	for b.Loop() {
		BuildMetricName("myapp", "http", "requests_total")
	}
}

func BenchmarkBuildTracerName(b *testing.B) {
	for b.Loop() {
		BuildTracerName("myapp", "orders")
	}
}

func BenchmarkJoinScope(b *testing.B) {
	for b.Loop() {
		JoinScope("parent", "child", '_')
	}
}
