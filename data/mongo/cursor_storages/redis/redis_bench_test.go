// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"

	cursredis "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func benchStorage(b *testing.B) *cursredis.Storage {
	b.Helper()
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })
	return cursredis.New(client)
}

func BenchmarkStorage_Store(b *testing.B) {
	s := benchStorage(b)
	ctx := b.Context()
	meta := testhelpers.SampleCursorMetadata()
	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkStorage_Load(b *testing.B) {
	s := benchStorage(b)
	ctx := b.Context()
	_ = s.Store(ctx, "bench-key", testhelpers.SampleCursorMetadata())
	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}
