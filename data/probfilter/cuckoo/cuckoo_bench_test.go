// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func BenchmarkFilter_Add(b *testing.B) {
	s := memory.New(memory.WithCapacity(1000000))
	f := cuckoo.New(s)
	ctx := b.Context()
	for b.Loop() {
		_ = f.Add(ctx, "bench-item")
	}
}

func BenchmarkFilter_MightExist(b *testing.B) {
	s := memory.New(memory.WithCapacity(1000000))
	f := cuckoo.New(s)
	ctx := b.Context()
	_ = f.Add(ctx, "bench-item")
	for b.Loop() {
		_, _ = f.MightExist(ctx, "bench-item")
	}
}
