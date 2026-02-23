// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
)

func BenchmarkStorage_Store(b *testing.B) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := b.Context()
	meta := sampleMetadata()

	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkStorage_Load(b *testing.B) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := b.Context()
	_ = s.Store(ctx, "bench-key", sampleMetadata())

	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}

func BenchmarkStorage_RunCleanup(b *testing.B) {
	s := memory.New(1 * time.Millisecond)
	defer s.Close()
	ctx := b.Context()

	for i := range 1000 {
		_ = s.Store(ctx, fmt.Sprintf("key-%d", i), sampleMetadata())
	}
	time.Sleep(5 * time.Millisecond)

	for b.Loop() {
		s.RunCleanup()
	}
}
