// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/memory"
)

func BenchmarkProvider_Allow(b *testing.B) {
	p := memory.New()
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Allow(ctx, "bench-key", 1000000, time.Minute)
	}
}

func BenchmarkProvider_Reset(b *testing.B) {
	p := memory.New()
	ctx := b.Context()
	_, _ = p.Allow(ctx, "bench-key", 1000000, time.Minute)

	for b.Loop() {
		_ = p.Reset(ctx, "bench-key")
	}
}
