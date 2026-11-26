// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
	redislimiter "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
)

// TestRedis_KeyLivesForOneWindow asserts the script's PEXPIRE on a real
// server: the sorted set expires one window after the last admission, so an
// idle client leaves nothing behind, and a rejection does not extend it.
func TestRedis_KeyLivesForOneWindow(t *testing.T) {
	t.Parallel()

	const period = 1500 * time.Millisecond
	client := redisClient(t)
	prefix := namespace(t)
	t.Cleanup(func() { deleteRedisKeys(client, prefix+"*") })
	store := redislimiter.New(client, redislimiter.WithKeyPrefix(prefix))
	key := prefix + ":idle-client"
	ctx := t.Context()

	_, err := store.Allow(ctx, "idle-client", 1, period)
	require.NoError(t, err)
	ttl, err := client.PTTL(ctx, key).Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "the window key must carry an expiry")
	require.LessOrEqual(t, ttl, period)

	_, err = store.Allow(ctx, "idle-client", 1, period)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
	members, err := client.ZCard(ctx, key).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), members, "a rejected request must not be recorded")

	require.Eventually(t, func() bool {
		n, err := client.Exists(ctx, key).Result()
		return err == nil && n == 0
	}, period+2*time.Second, 50*time.Millisecond, "the server never expired the idle window")
}

// TestRedis_SubSecondWindow guards the millisecond window: a sub-second period
// once became 0 and EXPIRE 0 deleted every recorded request, so nothing was
// ever limited.
func TestRedis_SubSecondWindow(t *testing.T) {
	t.Parallel()

	const period = 300 * time.Millisecond
	client := redisClient(t)
	prefix := namespace(t)
	t.Cleanup(func() { deleteRedisKeys(client, prefix+"*") })
	store := redislimiter.New(client, redislimiter.WithKeyPrefix(prefix))
	ctx := t.Context()

	start := time.Now()
	for range 2 {
		_, err := store.Allow(ctx, "burst", 2, period)
		require.NoError(t, err)
	}
	_, err := store.Allow(ctx, "burst", 2, period)
	if time.Since(start) < period {
		require.ErrorIs(t, err, limiters.ErrLimitExceeded, "a sub-second window must still limit")
	}

	require.Eventually(t, func() bool {
		_, err := store.Allow(ctx, "burst", 2, period)
		return err == nil
	}, 2*time.Second, 25*time.Millisecond, "the sub-second window never reopened")
}
