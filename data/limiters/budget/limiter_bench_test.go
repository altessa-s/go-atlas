// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget_test

import (
	"math"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

// BenchmarkLimiter_Allow measures the happy path: the budget is never
// exhausted, so every call increments the in-memory counter and succeeds.
func BenchmarkLimiter_Allow(b *testing.B) {
	cfg := &budget.Settings{Limit: math.MaxInt64, Period: time.Hour}
	l, err := budget.New(cfg, memory.New())
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := l.Allow(ctx, "bench-key"); err != nil {
			b.Fatalf("Allow: %v", err)
		}
	}
}
