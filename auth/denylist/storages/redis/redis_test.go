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
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupStore(tb testing.TB, opts ...redisstore.Option) (*redisstore.Store, *miniredis.Miniredis) {
	tb.Helper()
	client, mr := testhelpers.RedisClient(tb)
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

// TestStore_StreamValuesLiteralPrefix uses prefixes with Redis glob
// metacharacters: every revoked key is streamed, and keys that merely match
// the prefix as a pattern are not.
func TestStore_StreamValuesLiteralPrefix(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ prefix, lookalike string }{
		"brackets":  {prefix: "dl:[tenant]:", lookalike: "dl:t:x"},
		"star":      {prefix: "dl:*:", lookalike: "dl:any:x"},
		"question":  {prefix: "dl:?:", lookalike: "dl:a:x"},
		"backslash": {prefix: `dl:\:`, lookalike: "dl::x"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, mr := setupStore(t, redisstore.WithKeyPrefix(tc.prefix))
			ctx := t.Context()
			require.NoError(t, store.Revoke(ctx, "a"))
			require.NoError(t, store.Revoke(ctx, "b"))
			require.NoError(t, mr.Set(tc.lookalike, "1"))

			var got []string
			for key, err := range store.StreamValues(ctx) {
				require.NoError(t, err)
				got = append(got, key)
			}
			require.ElementsMatch(t, []string{"a", "b"}, got)
		})
	}
}

func TestStore_StreamValuesRejectsClusterAndRing(t *testing.T) {
	t.Parallel()
	for name, client := range map[string]goredis.UniversalClient{
		"cluster": goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{"127.0.0.1:1"}}),
		"ring":    goredis.NewRing(&goredis.RingOptions{Addrs: map[string]string{"a": "127.0.0.1:1"}}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(func() { _ = client.Close() })
			store := redisstore.New(client)

			var errs []error
			for key, err := range store.StreamValues(t.Context()) {
				require.Empty(t, key)
				errs = append(errs, err)
			}
			require.Len(t, errs, 1)
			require.ErrorIs(t, errs[0], redisstore.ErrUnsupportedClient)
		})
	}
}
