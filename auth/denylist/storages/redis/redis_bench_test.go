// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func benchStore(b *testing.B) *redisstore.Store {
	b.Helper()
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { _ = client.Close() })
	return redisstore.New(client)
}

func BenchmarkIsRevoked(b *testing.B) {
	store := benchStore(b)
	ctx := b.Context()
	_ = store.Revoke(ctx, "bench-key")

	b.ResetTimer()
	for b.Loop() {
		_, _ = store.IsRevoked(ctx, "bench-key")
	}
}

func BenchmarkRevoke(b *testing.B) {
	store := benchStore(b)
	ctx := b.Context()

	for b.Loop() {
		_ = store.Revoke(ctx, "bench-key")
	}
}
