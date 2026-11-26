// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/storagetest"

	idempredis "github.com/altessa-s/go-atlas/reliability/idempotency/storages/redis"
)

func TestKeeperTakeoverKeepsLockTTL(t *testing.T) {
	t.Parallel()
	s, mr := setupStorage(t)
	now := time.Now()
	mr.SetTime(now)
	ctx := t.Context()
	key := idempotency.ComposeKey("namespace", "", "k")
	wire := []byte(fmt.Sprintf(`{"status":%q,"locked_at":%q}`, idempotency.StatusInProgress, now.Add(-time.Second).Format(time.RFC3339Nano)))
	acquired, _, token, err := s.AttemptLockWithTTL(ctx, key, wire, time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	previous := &idempotency.State{}
	previous.SetLockToken(token)
	mr.SetTime(now.Add(2 * time.Millisecond))
	k := idempotency.New(s, idempotency.WithMaxLockDuration(time.Millisecond))
	acquired, _, err = k.AttemptLockWithOpts(ctx, "k", idempotency.AttemptLockOpts{LockTTL: time.Second})
	require.NoError(t, err)
	require.True(t, acquired)
	require.True(t, mr.Exists(idempredis.DefaultKeyPrefix+key))
	ttl, err := s.Client().PTTL(ctx, idempredis.DefaultKeyPrefix+key).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, time.Second)
	require.ErrorIs(t, k.Complete(ctx, "k", "stale", previous), idempotency.ErrLockStolen)
}

func TestKeeperUsesRedisClockForTakeover(t *testing.T) {
	t.Parallel()
	s, mr := setupStorage(t)
	mr.SetTime(time.Now())
	storagetest.KeeperUsesStorageClock(t, s)
}
