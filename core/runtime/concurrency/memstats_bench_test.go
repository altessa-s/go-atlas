// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"runtime"
	"testing"
	"time"
)

func BenchmarkGetCachedMemStats(b *testing.B) {
	InvalidateMemStatsCache()
	SetMemStatsCacheTTL(time.Second)

	b.ResetTimer()
	for b.Loop() {
		_ = getCachedMemStats()
	}
}

func BenchmarkRuntimeReadMemStats(b *testing.B) {
	var m runtime.MemStats
	b.ResetTimer()
	for b.Loop() {
		runtime.ReadMemStats(&m)
	}
}
