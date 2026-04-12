// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kvstore_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"
	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func BenchmarkJSONStorage_Store(b *testing.B) {
	backend := newMockBackend()
	s := kvstore.NewJSONStorage(backend, "bench")
	ctx := b.Context()
	meta := testhelpers.SampleCursorMetadata()

	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkJSONStorage_Load(b *testing.B) {
	backend := newMockBackend()
	s := kvstore.NewJSONStorage(backend, "bench")
	ctx := b.Context()
	_ = s.Store(ctx, "bench-key", testhelpers.SampleCursorMetadata())

	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}
