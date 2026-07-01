// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupStore(tb testing.TB, opts ...redisstore.Option) (*redisstore.Store, *miniredis.Miniredis) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { _ = client.Close() })
	return redisstore.New(client, opts...), mr
}

func TestStore_IsDataLoader(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)
	var _ probfilter.DataLoader = store
}

func TestStore_RevokeThenIsRevoked(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)
	ctx := t.Context()

	require.NoError(t, store.Revoke(ctx, "jti-1"))

	revoked, err := store.IsRevoked(ctx, "jti-1")
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestStore_UnknownKeyNotRevoked(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)

	revoked, err := store.IsRevoked(t.Context(), "missing")
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestStore_RevokeUntilExpires(t *testing.T) {
	t.Parallel()
	store, mr := setupStore(t)
	ctx := t.Context()

	const ttl = time.Minute
	require.NoError(t, store.RevokeUntil(ctx, "jti-ttl", ttl))

	revoked, err := store.IsRevoked(ctx, "jti-ttl")
	require.NoError(t, err)
	require.True(t, revoked)

	mr.FastForward(ttl + time.Second)

	revoked, err = store.IsRevoked(ctx, "jti-ttl")
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestStore_RevokeUntilNonPositiveIsNoop(t *testing.T) {
	t.Parallel()
	store, mr := setupStore(t)
	ctx := t.Context()

	require.NoError(t, store.RevokeUntil(ctx, "jti-noop", 0))
	require.NoError(t, store.RevokeUntil(ctx, "jti-noop", -time.Second))

	revoked, err := store.IsRevoked(ctx, "jti-noop")
	require.NoError(t, err)
	require.False(t, revoked)
	require.False(t, mr.Exists(redisstore.DefaultKeyPrefix+"jti-noop"))
}

func TestStore_Restore(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)
	ctx := t.Context()

	require.NoError(t, store.Revoke(ctx, "jti-restore"))
	require.NoError(t, store.Restore(ctx, "jti-restore"))

	revoked, err := store.IsRevoked(ctx, "jti-restore")
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestStore_StreamValuesYieldsBareKeys(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)
	ctx := t.Context()

	want := []string{"a", "b", "c", "d"}
	for _, k := range want {
		require.NoError(t, store.Revoke(ctx, k))
	}

	var got []string
	for key, err := range store.StreamValues(ctx) {
		require.NoError(t, err)
		got = append(got, key)
	}
	require.ElementsMatch(t, want, got)
}

func TestStore_StreamValuesRespectsPrefix(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t, redisstore.WithKeyPrefix("dl:"))
	ctx := t.Context()

	require.NoError(t, store.Revoke(ctx, "only"))

	var got []string
	for key, err := range store.StreamValues(ctx) {
		require.NoError(t, err)
		got = append(got, key)
	}
	require.ElementsMatch(t, []string{"only"}, got)
}

func TestStore_CountUnknown(t *testing.T) {
	t.Parallel()
	store, _ := setupStore(t)

	n, err := store.Count(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(-1), n)
}
