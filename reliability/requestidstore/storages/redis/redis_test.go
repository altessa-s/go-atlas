// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
)

func TestConformance(t *testing.T) {
	t.Parallel()
	requestidtest.Run(t, func(t *testing.T) requestid.Store { client, _ := redistest.RedisClient(t); return redis.New(client) })
}

func TestPhysicalTTL(t *testing.T) {
	t.Parallel()
	client, server := redistest.RedisClient(t)
	const grace = 5 * time.Minute
	store := redis.New(client, redis.WithCleanupGrace(grace))
	now := requestidtest.Base
	entry := requestid.Entry{State: requestid.StatePending, Token: "first", ExpiresAt: now.Add(time.Second)}
	key := requestidtest.Key(t.Name())
	_, ok, err := store.Reserve(t.Context(), key, entry, now)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, time.Second+grace, server.TTL(redis.DefaultKeyPrefix+key))
	entry.State = requestid.StateCompleted
	entry.ExpiresAt = now.Add(time.Hour)
	require.NoError(t, store.Complete(t.Context(), key, entry.Token, entry, now))
	require.Equal(t, time.Hour+grace, server.TTL(redis.DefaultKeyPrefix+key))
	// Redis has passed the original TTL plus one minute, while the caller still sees a live result.
	server.FastForward(time.Hour + 2*time.Minute)
	got, won, err := store.Reserve(t.Context(), key, entry, entry.ExpiresAt.Add(-time.Second))
	require.NoError(t, err)
	require.False(t, won)
	require.Equal(t, requestid.StateCompleted, got.State)
	_, won, err = store.Reserve(t.Context(), key, entry, entry.ExpiresAt)
	require.NoError(t, err)
	require.True(t, won)
}
