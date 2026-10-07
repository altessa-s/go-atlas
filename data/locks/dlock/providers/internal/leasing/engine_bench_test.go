// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing_test

import (
	"context"
	"testing"
	"time"
)

// BenchmarkLockRelease measures one acquisition and release through the
// engine, the store being in memory.
func BenchmarkLockRelease(b *testing.B) {
	e := newEngine(b, newMemStore(time.Minute))
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		lk, err := e.Lock(ctx, "k")
		if err != nil {
			b.Fatal(err)
		}
		if err := lk.Release(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
