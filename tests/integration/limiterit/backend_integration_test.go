// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterit_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"

	natslimiter "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
	redislimiter "github.com/altessa-s/go-atlas/data/limiters/storages/redis"
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
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
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
		require.ErrorIs(t, err, storages.ErrLimitExceeded, "a sub-second window must still limit")
	}

	require.Eventually(t, func() bool {
		_, err := store.Allow(ctx, "burst", 2, period)
		return err == nil
	}, 2*time.Second, 25*time.Millisecond, "the sub-second window never reopened")
}

// TestNATS_ResetRecreatesDeletedKey asserts the CAS path across a Reset on a
// real bucket: Reset leaves a delete marker, and the next admission must
// create the key again rather than fail on the tombstone's revision — from
// another replica too.
func TestNATS_ResetRecreatesDeletedKey(t *testing.T) {
	t.Parallel()

	replica := openNATS(t)
	first, second := replica(), replica()
	ctx := t.Context()

	for range 2 {
		_, err := first.Allow(ctx, "tombstoned", 2, time.Minute)
		require.NoError(t, err)
	}
	require.NoError(t, second.Reset(ctx, "tombstoned"))
	require.NoError(t, second.Reset(ctx, "tombstoned"), "resetting a deleted key again")

	info, err := first.Allow(ctx, "tombstoned", 2, time.Minute)
	require.NoError(t, err, "admission after a reset by another replica")
	require.Equal(t, int64(1), info.Remaining)
}

// TestNATS_BucketTTLMismatchIsRefused asserts that a replica configured with
// another key TTL cannot silently re-time a bucket every replica shares.
func TestNATS_BucketTTLMismatchIsRefused(t *testing.T) {
	t.Parallel()

	bucket := namespace(t)
	admin, err := jetstream.New(natsConn(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.DeleteKeyValue(context.Background(), bucket) })

	_, err = natslimiter.New(admin, natslimiter.WithBucket(bucket), natslimiter.WithMaxAge(time.Hour))
	require.NoError(t, err)

	_, err = natslimiter.New(admin, natslimiter.WithBucket(bucket), natslimiter.WithMaxAge(time.Minute))
	require.ErrorIs(t, err, natslimiter.ErrBucketTTLMismatch)
}
