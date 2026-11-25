// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	factory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	bloom "github.com/altessa-s/go-atlas/factory/cache/probfilter/bloom/storages/redis"
	cuckoo "github.com/altessa-s/go-atlas/factory/cache/probfilter/cuckoo/storages/redis"
	redisfactory "github.com/altessa-s/go-atlas/factory/infrastructure/redis"
)

// Exercise both public storage builders, including the borrowed client's lifetime.
func checkBuilders(t *testing.T, client redis.UniversalClient, sentinel *redis.ClusterClient, want error) {
	t.Helper()
	bf, cf := bloom.Builder(client), cuckoo.Builder(client)
	if sentinel != nil {
		bf, cf = bloom.SentinelBuilder(sentinel), cuckoo.SentinelBuilder(sentinel)
	}
	// Both builders use the same owner and discovery state.
	t.Run("Bloom", func(t *testing.T) {
		b, err := bf(t.Context(), factory.BloomStorageSpec{ExpectedItems: 100})
		require.ErrorIs(t, err, want)
		if err == nil {
			require.NoError(t, b.Close(t.Context()))
		}
	})
	t.Run("Cuckoo", func(t *testing.T) {
		c, err := cf(t.Context(), factory.CuckooStorageSpec{Capacity: 100})
		require.ErrorIs(t, err, want)
		if err == nil {
			require.NoError(t, c.Close(t.Context()))
		}
	})
}

func servePolicy(t testing.TB, mr *miniredis.Miniredis, policy string) {
	t.Helper()
	require.NoError(t, mr.Server().Register("CONFIG", func(c *server.Peer, _ string, _ []string) {
		if policy == "denied" {
			c.WriteError("NOPERM CONFIG denied")
			return
		}
		c.WriteMapLen(1)
		c.WriteBulk("maxmemory-policy")
		c.WriteBulk(policy)
	}))
}

func TestStorageBuildersBorrowManagedClient(t *testing.T) {
	t.Parallel()
	_, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")
	servePolicy(t, mr, "noeviction")
	owner, err := redisfactory.New(&redisconfig.Config{Hosts: []string{mr.Addr()}}).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	checkBuilders(t, owner.Client(), nil, nil)
	require.NoError(t, owner.Client().Ping(t.Context()).Err())
}

func TestStorageBuildersRejectIncompleteTopology(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"replica", "cluster endpoint", "decorated cluster", "ring growth"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			policy := "noeviction"
			if kind == "decorated cluster" {
				policy = "allkeys-lru"
			}
			servePolicy(t, mr, policy)
			switch kind {
			case "replica":
				redistest.ServeTopology(mr, "slave", "0")
				checkBuilders(t, client, nil, probfilter.ErrUnverifiableTopology)
			case "cluster endpoint":
				redistest.ServeTopology(mr, "master", "1")
				checkBuilders(t, client, nil, probfilter.ErrUnverifiableTopology)
			case "decorated cluster":
				c := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{mr.Addr()}})
				defer c.Close()
				checkBuilders(t, struct{ redis.UniversalClient }{c}, nil, probfilter.ErrUnverifiableTopology)
				require.Zero(t, mr.CommandCount())
			case "ring growth":
				c := redis.NewRing(&redis.RingOptions{Addrs: map[string]string{"safe": mr.Addr()}})
				defer c.Close()
				c.SetAddrs(map[string]string{"safe": mr.Addr(), "offline": "127.0.0.1:1"})
				checkBuilders(t, c, nil, probfilter.ErrUnverifiableTopology)
			}
		})
	}
}

// shardsHook supplies the synchronous CLUSTER SHARDS response independently
// of the driver's slot cache to model a change between discovery and routing.
type shardsHook struct {
	shards []redis.ClusterShard
	err    error
}

func (h shardsHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h shardsHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h shardsHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if reply, ok := cmd.(*redis.ClusterShardsCmd); ok {
			reply.SetVal(h.shards)
			return h.err
		}
		return next(ctx, cmd)
	}
}

func TestStorageBuildersRequireFreshClusterPrimaries(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"safe", "denied and unsafe", "denied and safe", "discovery failure", "added unsafe primary", "identity mismatch"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			_, a := redistest.RedisClient(t)
			_, b := redistest.RedisClient(t)
			slots := []redis.ClusterSlot{{Start: 0, End: 8191, Nodes: []redis.ClusterNode{{Addr: a.Addr()}}}, {Start: 8192, End: 16383, Nodes: []redis.ClusterNode{{Addr: b.Addr()}}}}
			var stale atomic.Bool
			cluster := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{a.Addr()}, ClusterSlots: func(context.Context) ([]redis.ClusterSlot, error) {
				if stale.Load() {
					return nil, context.DeadlineExceeded
				}
				if mode == "added unsafe primary" || mode == "discovery failure" {
					return slots[:1], nil
				}
				return slots, nil
			}})
			defer cluster.Close()
			var hook shardsHook
			for i, mr := range []*miniredis.Miniredis{a, b} {
				id := mr.Addr()
				mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
					if cmd == "CLUSTER" && len(args) == 1 && strings.EqualFold(args[0], "myid") {
						c.WriteBulk(id)
						return true
					}
					return false
				})
				policy := "noeviction"
				if i == 0 && strings.HasPrefix(mode, "denied") {
					policy = "denied"
				}
				if i == 1 && (mode == "denied and unsafe" || mode == "added unsafe primary" || mode == "discovery failure") {
					policy = "allkeys-lru"
				}
				servePolicy(t, mr, policy)
				node := redis.Node{ID: id, Role: "master", Health: "online"}
				if i == 1 && mode == "identity mismatch" {
					node.ID = "replacement"
				}
				hook.shards = append(hook.shards, redis.ClusterShard{Nodes: []redis.Node{node}})
			}
			// Warm the routing cache before forcing reload failure.
			require.NoError(t, cluster.ForEachMaster(t.Context(), func(context.Context, *redis.Client) error { return nil }))
			stale.Store(true)
			var want error
			switch mode {
			case "denied and unsafe":
				want = probfilter.ErrUnsafeEvictionPolicy
			case "discovery failure":
				hook.err = context.DeadlineExceeded
				want = probfilter.ErrUnverifiableTopology
			case "added unsafe primary", "identity mismatch":
				want = probfilter.ErrUnverifiableTopology
			}
			cluster.AddHook(hook)
			checkBuilders(t, cluster, nil, want)
		})
	}
}

func TestStorageBuildersDiscoverSentinelPrimary(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"latency", "random", "forced cluster", "unsafe", "discovery failure", "primary replaced"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			_, primary := redistest.RedisClient(t)
			_, sentinel := redistest.RedisClient(t)
			host, port, err := net.SplitHostPort(primary.Addr())
			require.NoError(t, err)
			redistest.ServeTopology(primary, "master", "0")
			var changed atomic.Bool
			require.NoError(t, sentinel.Server().Register("SENTINEL", func(c *server.Peer, _ string, args []string) {
				if strings.EqualFold(args[0], "get-master-addr-by-name") {
					if changed.Load() && mode == "discovery failure" {
						c.WriteError("ERR unavailable")
						return
					}
					c.WriteLen(2)
					c.WriteBulk(host)
					if changed.Load() && mode == "primary replaced" {
						c.WriteBulk("1")
					} else {
						c.WriteBulk(port)
					}
					return
				}
				c.WriteLen(0)
			}))
			policy := "noeviction"
			if mode == "unsafe" {
				policy = "allkeys-random"
			}
			servePolicy(t, primary, policy)
			var client *redis.ClusterClient
			if mode == "forced cluster" {
				raw := redis.NewUniversalClient(&redis.UniversalOptions{MasterName: "test", Addrs: []string{sentinel.Addr()}, IsClusterMode: true})
				client = raw.(*redis.ClusterClient)
				defer raw.Close()
			} else {
				owner, err := redisfactory.New(&redisconfig.Config{MasterName: "test", Hosts: []string{sentinel.Addr()}, RouteByLatency: mode != "random", RouteRandomly: mode == "random"}).SkipStartupPing().Build(t.Context())
				require.NoError(t, err)
				defer owner.Close()
				client = owner.SentinelCluster()
				require.Same(t, owner.Client(), client)
			}
			_, err = client.MasterForKey(t.Context(), "key")
			require.NoError(t, err)
			changed.Store(true)
			var want error
			switch mode {
			case "unsafe":
				want = probfilter.ErrUnsafeEvictionPolicy
			case "primary replaced", "discovery failure":
				want = probfilter.ErrUnverifiableTopology
			}
			checkBuilders(t, client, client, want)
		})
	}
}
