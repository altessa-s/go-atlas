// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cursredis "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
	mongohelpers "github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func benchStorage(b *testing.B) *cursredis.Storage {
	b.Helper()
	client, _ := testhelpers.RedisClient(b)
	return cursredis.New(client)
}

func BenchmarkStorage_Store(b *testing.B) {
	s := benchStorage(b)
	ctx := b.Context()
	meta := mongohelpers.SampleCursorMetadata()
	for b.Loop() {
		_ = s.Store(ctx, "bench-key", meta)
	}
}

func BenchmarkStorage_Load(b *testing.B) {
	s := benchStorage(b)
	ctx := b.Context()
	_ = s.Store(ctx, "bench-key", mongohelpers.SampleCursorMetadata())
	for b.Loop() {
		_, _ = s.Load(ctx, "bench-key")
	}
}
