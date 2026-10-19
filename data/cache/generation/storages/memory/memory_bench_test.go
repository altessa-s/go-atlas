// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
)

func BenchmarkStore_Current(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	for b.Loop() {
		_, _ = s.Current(ctx, "catalog")
	}
}

func BenchmarkStore_Bump(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	for b.Loop() {
		_, _ = s.Bump(ctx, "catalog")
	}
}
