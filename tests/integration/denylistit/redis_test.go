// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylistit_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestDirectReplica(t *testing.T) {
	t.Parallel()
	addr := os.Getenv("REDIS_REPLICA_ADDR")
	if addr == "" {
		t.Skip("set REDIS_REPLICA_ADDR to a Redis replica")
	}
	checkReplica(t, goredis.NewClient(&goredis.Options{Addr: addr}))
}

func TestSentinelReplicaOnly(t *testing.T) {
	t.Parallel()
	addr, master := os.Getenv("REDIS_SAFE_SENTINEL_ADDR"), os.Getenv("REDIS_SENTINEL_MASTER_NAME")
	if addr == "" || master == "" {
		t.Skip("set REDIS_SAFE_SENTINEL_ADDR and REDIS_SENTINEL_MASTER_NAME for a Sentinel with a replica")
	}
	checkReplica(t, goredis.NewFailoverClient(&goredis.FailoverOptions{MasterName: master, SentinelAddrs: []string{addr}, ReplicaOnly: true}))
}

func checkReplica(t testing.TB, client *goredis.Client) {
	t.Helper()
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	role, err := client.Do(ctx, "ROLE").Slice()
	require.NoError(t, err)
	require.NotEmpty(t, role)
	require.Equal(t, "slave", role[0])
	store, err := redisstore.New(client, redisstore.WithStrictExactReads())
	require.NoError(t, err)
	_, err = store.IsRevoked(ctx, "replica-probe")
	require.ErrorIs(t, err, redisstore.ErrReplicaReadNotAllowed)
	var count int
	for key, err := range store.RawSource().StreamValues(ctx) {
		require.Empty(t, key)
		require.ErrorIs(t, err, redisstore.ErrUnsupportedClient)
		count++
	}
	require.Equal(t, 1, count)
}
