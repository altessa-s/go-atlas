// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/storages/freecache"
)

func BenchmarkProvider_Save(b *testing.B) {
	p := freecache.New()
	ctx := b.Context()
	value := []byte("benchmark-value")

	b.ResetTimer()
	for b.Loop() {
		key := "bench-key"
		_ = p.Save(ctx, key, value, 10*time.Second)
	}
}

func BenchmarkProvider_Get(b *testing.B) {
	p := freecache.New()
	ctx := b.Context()
	key := "bench-key"
	value := []byte("benchmark-value")

	_ = p.Save(ctx, key, value, 10*time.Second)

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Get(ctx, key)
	}
}

func BenchmarkProvider_Exists(b *testing.B) {
	p := freecache.New()
	ctx := b.Context()
	key := "bench-key"
	value := []byte("benchmark-value")

	_ = p.Save(ctx, key, value, 10*time.Second)

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Exists(ctx, key)
	}
}

// BenchmarkProvider_Exists_LargeValue checks existence of a 4 KiB value;
// the check must not pay for copying the value.
func BenchmarkProvider_Exists_LargeValue(b *testing.B) {
	p := freecache.New(freecache.WithMaxSize(16 << 20))
	ctx := b.Context()
	_ = p.Save(ctx, "bench-key", make([]byte, 4<<10), 10*time.Second)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = p.Exists(ctx, "bench-key")
	}
}
