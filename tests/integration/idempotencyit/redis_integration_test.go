// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	idemredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// redisFixture is a Redis storage plus the raw client and the full key of
// the storage's "k", so a test can read what the Lua scripts left on the
// server.
type redisFixture struct {
	storage *idemredis.Storage
	client  *goredis.Client
	rawKey  string
}

func newRedisFixture(tb testing.TB, ttl time.Duration) redisFixture {
	tb.Helper()
	client := livebackend.ConnectRedis(tb)
	prefix := livebackend.RedisPrefix(tb, client, "idemit")
	return redisFixture{
		storage: idemredis.New(client, idemredis.WithKeyPrefix(prefix), idemredis.WithTTL(ttl)),
		client:  client,
		rawKey:  prefix + "k",
	}
}

// pttl returns the remaining lifetime of the raw key on the server.
func (f redisFixture) pttl(tb testing.TB) time.Duration {
	tb.Helper()
	d, err := f.client.PTTL(tb.Context(), f.rawKey).Result()
	require.NoError(tb, err)
	return d
}

// raw returns the bytes stored under the raw key.
func (f redisFixture) raw(tb testing.TB) []byte {
	tb.Helper()
	v, err := f.client.Get(tb.Context(), f.rawKey).Bytes()
	require.NoError(tb, err)
	return v
}

// TestRedis_LuaWritesCarryTheStorageTTL pins the PX argument of the
// compare-and-set script: Complete and Steal pass the storage TTL to Lua as
// a string of milliseconds, and the key must come back with that lifetime —
// not the per-call lock TTL, and not without any expiry.
func TestRedis_LuaWritesCarryTheStorageTTL(t *testing.T) {
	t.Parallel()
	const storageTTL = 10 * time.Minute
	f := newRedisFixture(t, storageTTL)
	ctx := t.Context()

	acquired, _, _, err := f.storage.AttemptLockWithTTL(ctx, "k", payload("lock"), 5*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	require.LessOrEqual(t, f.pttl(t), 5*time.Second, "SET NX must apply the per-call TTL")
	require.Positive(t, f.pttl(t))

	fresh, err := f.storage.Steal(ctx, "k", payload("lock"), payload("reclaimed"))
	require.NoError(t, err)
	require.Greater(t, f.pttl(t), storageTTL-time.Minute, "Steal must renew the key with the storage TTL")
	require.LessOrEqual(t, f.pttl(t), storageTTL)

	// Shorten the key on the server, so the renewal by Complete is visible.
	require.NoError(t, f.client.PExpire(ctx, f.rawKey, 5*time.Second).Err())
	require.NoError(t, f.storage.Complete(ctx, "k", payload("done"), fresh))
	require.Greater(t, f.pttl(t), storageTTL-time.Minute, "Complete must renew the key with the storage TTL")
	require.Equal(t, payload("done"), f.raw(t), "Lua must store the bytes unchanged")
}

// TestRedis_RefusedLuaWritesLeaveTheKeyUntouched pins the mismatch branches
// of both scripts: a refused Complete, Steal or Release changes neither the
// value nor the remaining lifetime of the key.
func TestRedis_RefusedLuaWritesLeaveTheKeyUntouched(t *testing.T) {
	t.Parallel()
	f := newRedisFixture(t, 10*time.Minute)
	ctx := t.Context()

	acquired, _, _, err := f.storage.AttemptLockWithTTL(ctx, "k", payload("lock"), 30*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	before := f.pttl(t)

	// Values that differ from the stored one only after a NUL byte or in a
	// byte above 0x7f: a comparison that stops at NUL or decodes text would
	// take them for a match.
	near := [][]byte{
		payload("lock")[:1],
		append(payload("lock"), 0x00),
		append(payload("lock")[:len(payload("lock"))-1], 0x81),
	}
	for _, v := range near {
		require.ErrorIs(t, f.storage.Complete(ctx, "k", payload("done"), v), storages.ErrLockStolen)
		_, err := f.storage.Steal(ctx, "k", v, payload("reclaimed"))
		require.ErrorIs(t, err, storages.ErrLockStolen)
		require.ErrorIs(t, f.storage.Release(ctx, "k", v), storages.ErrLockStolen)
	}

	require.Equal(t, payload("lock"), f.raw(t))
	after := f.pttl(t)
	require.LessOrEqual(t, after, before, "a refused write must not renew the key")
	require.Greater(t, after, 20*time.Second)
}

// TestRedis_LuaOnMissingKeyWritesNothing pins the missing-key branch of both
// scripts: GET returns false in Lua, the scripts report it, and nothing is
// written — in particular no key without an expiry.
func TestRedis_LuaOnMissingKeyWritesNothing(t *testing.T) {
	t.Parallel()
	f := newRedisFixture(t, 10*time.Minute)
	ctx := t.Context()

	require.ErrorIs(t, f.storage.Complete(ctx, "k", payload("done"), payload("lock")), storages.ErrLockStolen)
	_, err := f.storage.Steal(ctx, "k", payload("lock"), payload("reclaimed"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
	require.ErrorIs(t, f.storage.Release(ctx, "k", payload("lock")), storages.ErrLockStolen)

	n, err := f.client.Exists(ctx, f.rawKey).Result()
	require.NoError(t, err)
	require.Zero(t, n, "a refused script must not create the key")
}

// TestRedis_ReleaseDeletesOnlyTheOwnedKey pins the release script's match
// branch against the raw server state: the key is gone, not emptied.
func TestRedis_ReleaseDeletesOnlyTheOwnedKey(t *testing.T) {
	t.Parallel()
	f := newRedisFixture(t, 10*time.Minute)
	ctx := t.Context()

	token := lock(t, f.storage, "k", payload("lock"))
	require.NoError(t, f.storage.Release(ctx, "k", token))

	n, err := f.client.Exists(ctx, f.rawKey).Result()
	require.NoError(t, err)
	require.Zero(t, n)
}

// TestRedis_CompleteWithoutTokenUsesTheStorageTTL pins the unguarded
// overwrite: a plain SET with the storage TTL.
func TestRedis_CompleteWithoutTokenUsesTheStorageTTL(t *testing.T) {
	t.Parallel()
	const storageTTL = 10 * time.Minute
	f := newRedisFixture(t, storageTTL)

	require.NoError(t, f.storage.Complete(t.Context(), "k", payload("forced"), nil))
	require.Equal(t, payload("forced"), f.raw(t))
	require.Greater(t, f.pttl(t), storageTTL-time.Minute)
}
