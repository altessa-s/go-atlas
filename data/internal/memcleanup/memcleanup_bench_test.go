// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memcleanup_test

import (
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/internal/memcleanup"
)

// BenchmarkSweep measures a sweep over 10k entries with ~10% expired,
// matching the pre-allocation heuristic of the implementation.
func BenchmarkSweep(b *testing.B) {
	base := time.Now()
	future := base.Add(time.Hour)
	past := base.Add(-time.Hour)

	var mu sync.RWMutex
	entries := make(map[int]*item, 10000)

	for b.Loop() {
		b.StopTimer()
		for i := range 10000 {
			exp := future
			if i%10 == 0 {
				exp = past
			}
			entries[i] = &item{expiresAt: exp}
		}
		b.StartTimer()

		now := time.Now()
		memcleanup.Sweep(&mu, entries, func(it *item) bool {
			return now.After(it.expiresAt)
		})
	}
}
