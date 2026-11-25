// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
)

func benchStore(b *testing.B) *redisstore.Store {
	b.Helper()
	client, _ := redistest.RedisClient(b)
	store, err := redisstore.New(client)
	require.NoError(b, err)
	return store
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
