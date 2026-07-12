// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"testing"
	"time"
)

func BenchmarkProvider_Save(b *testing.B) {
	p, err := New(1024)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx := b.Context()
	value := []byte("benchmark-value")

	b.ReportAllocs()
	for b.Loop() {
		_ = p.Save(ctx, "bench-key", value, time.Hour)
	}
}

func BenchmarkProvider_Get(b *testing.B) {
	p, err := New(1024)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx := b.Context()
	if err := p.Save(ctx, "bench-key", []byte("benchmark-value"), time.Hour); err != nil {
		b.Fatalf("Save: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = p.Get(ctx, "bench-key")
	}
}
