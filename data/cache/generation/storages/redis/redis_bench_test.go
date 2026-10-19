// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// BenchmarkStore_CurrentLocal measures the hot path: a generation reused
// within the staleness bound.
func BenchmarkStore_CurrentLocal(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)
	s := redis.New(client)
	ctx := b.Context()
	_, _ = s.Current(ctx, "catalog")
	for b.Loop() {
		_, _ = s.Current(ctx, "catalog")
	}
}

func BenchmarkStore_CurrentRedis(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)
	s := redis.New(client, redis.WithMaxStaleness(0))
	ctx := b.Context()
	for b.Loop() {
		_, _ = s.Current(ctx, "catalog")
	}
}
