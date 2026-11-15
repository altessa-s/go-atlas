// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	idempredis "github.com/altessa-s/go-atlas/reliability/idempotency/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestStorage_ReleaseOnlyByOwner(t *testing.T) {
	t.Parallel()
	s, mr := setupStorage(t)
	ctx := t.Context()

	ok, _, token, err := s.AttemptLock(ctx, "k", []byte("v1"))
	require.NoError(t, err)
	require.True(t, ok)

	require.ErrorIs(t, s.Release(ctx, "", token), storages.ErrEmptyKey)
	require.ErrorIs(t, s.Release(ctx, "k", nil), storages.ErrMissingLockState)
	require.ErrorIs(t, s.Release(ctx, "k", []byte("other")), storages.ErrLockStolen)
	require.Len(t, mr.Keys(), 1, "a foreign token must not release the key")

	require.NoError(t, s.Release(ctx, "k", token))
	require.Empty(t, mr.Keys())
	require.ErrorIs(t, s.Release(ctx, "k", token), storages.ErrLockStolen, "a released key cannot be released again")
}

func TestStorage_PrefixAndTTLOptions(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	prefix := " idem: "
	s := idempredis.New(client,
		idempredis.WithKeyPrefix(&prefix), idempredis.WithKeyPrefix(""), idempredis.WithKeyPrefix((*string)(nil)),
		idempredis.WithTTL(time.Minute), idempredis.WithTTL(0))
	ctx := t.Context()

	ok, _, _, err := s.AttemptLock(ctx, "k", []byte("v"))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, mr.Exists("idem:k"), "keys carry the trimmed prefix; blank prefixes are ignored")
	require.Equal(t, time.Minute, mr.TTL("idem:k"), "a non-positive TTL option keeps the earlier value")

	ok, _, _, err = s.AttemptLockWithTTL(ctx, "short", []byte("v"), 5*time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 5*time.Second, mr.TTL("idem:short"), "the per-call TTL overrides the configured one")
}

func TestStorage_ErrorsWhenRedisUnavailable(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	s := idempredis.New(client)
	ctx := t.Context()
	mr.Close()

	_, _, _, err := s.AttemptLock(ctx, "k", []byte("v"))
	require.Error(t, err)
	require.Error(t, s.Complete(ctx, "k", []byte("v"), nil))
	require.Error(t, s.Complete(ctx, "k", []byte("v"), []byte("tok")))
	require.Error(t, s.Delete(ctx, "k"))
	require.Error(t, s.Release(ctx, "k", []byte("tok")))
}

func TestStorage_EmptyKeyRejected(t *testing.T) {
	t.Parallel()
	s, _ := setupStorage(t)
	ctx := t.Context()
	_, _, _, err := s.AttemptLockWithTTL(ctx, "", []byte("v"), time.Second)
	require.ErrorIs(t, err, storages.ErrEmptyKey)
	require.ErrorIs(t, s.Delete(ctx, ""), storages.ErrEmptyKey)
	require.ErrorIs(t, s.Complete(ctx, "", nil, nil), storages.ErrEmptyKey)
}
