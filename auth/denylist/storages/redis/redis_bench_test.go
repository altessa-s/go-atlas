// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
)

func benchStore(b *testing.B) *redisstore.Store {
	b.Helper()
	client, _ := testhelpers.RedisClient(b)
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
