// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	factory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	bloom "github.com/altessa-s/go-atlas/factory/cache/probfilter/bloom/storages/redis"
	cuckoo "github.com/altessa-s/go-atlas/factory/cache/probfilter/cuckoo/storages/redis"
	redisfactory "github.com/altessa-s/go-atlas/factory/infrastructure/redis"
)

func requiredEnv(t testing.TB, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("set %s to run this topology test", key)
	}
	return value
}

// Each address must name a reachable endpoint of the indicated topology.
// The checks only read topology and configuration.
func TestDirectClientRejectsClusterNodeAndReplica(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"REDIS_CLUSTER_NODE_ADDR", "REDIS_REPLICA_ADDR"} {
		t.Run(env, func(t *testing.T) {
			t.Parallel()
			owner, err := redisfactory.New(&redisconfig.Config{Hosts: []string{requiredEnv(t, env)}}).Build(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = owner.Close() })
			client := owner.Client().(*redis.Client)
			role, err := client.Do(t.Context(), "ROLE").Slice()
			require.NoError(t, err)
			require.NotEmpty(t, role)
			if env == "REDIS_REPLICA_ADDR" {
				require.Equal(t, "slave", role[0])
			} else {
				info, err := client.Info(t.Context(), "cluster").Result()
				require.NoError(t, err)
				require.Contains(t, info, "cluster_enabled:1")
			}
			_, err = bloom.Builder(client)(t.Context(), factory.BloomStorageSpec{})
			require.ErrorIs(t, err, probfilter.ErrUnverifiableTopology)
			_, err = cuckoo.Builder(client)(t.Context(), factory.CuckooStorageSpec{})
			require.ErrorIs(t, err, probfilter.ErrUnverifiableTopology)
		})
	}
}

// Use a safe cluster or Sentinel deployment with permission to read CONFIG.
func TestLivePrimaryDiscovery(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"REDIS_SAFE_CLUSTER_ADDR", "REDIS_SAFE_SENTINEL_ADDR"} {
		t.Run(env, func(t *testing.T) {
			t.Parallel()
			opts := &redis.UniversalOptions{Addrs: []string{requiredEnv(t, env)}, IsClusterMode: true}
			if env == "REDIS_SAFE_SENTINEL_ADDR" {
				opts.MasterName = requiredEnv(t, "REDIS_SENTINEL_MASTER_NAME")
			}
			client := redis.NewUniversalClient(opts).(*redis.ClusterClient)
			t.Cleanup(func() { _ = client.Close() })
			bf, cf := bloom.Builder(client), cuckoo.Builder(client)
			if opts.MasterName != "" {
				bf, cf = bloom.SentinelBuilder(client), cuckoo.SentinelBuilder(client)
			}
			b, err := bf(t.Context(), factory.BloomStorageSpec{ExpectedItems: 100})
			require.NoError(t, err)
			require.NoError(t, b.Close(t.Context()))
			c, err := cf(t.Context(), factory.CuckooStorageSpec{Capacity: 100})
			require.NoError(t, err)
			require.NoError(t, c.Close(t.Context()))
		})
	}
}
