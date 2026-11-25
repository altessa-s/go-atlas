// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/storagetest"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
	goredis "github.com/redis/go-redis/v9"
)

// topologyHook counts the two verification commands independently of Lua and
// connection setup. Tests can pause ROLE to keep a failed check in flight.
type topologyHook struct {
	role, info atomic.Int64
	onRole     func() error
}

func (*topologyHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (*topologyHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *topologyHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		switch cmd.Name() {
		case "role":
			h.role.Add(1)
			if h.onRole != nil {
				return h.onRole()
			}
		case "info":
			if args := cmd.Args(); len(args) == 2 && args[1] == "cluster" {
				h.info.Add(1)
			}
		}
		return next(ctx, cmd)
	}
}

func (h *topologyHook) counts() [2]int64 { return [2]int64{h.role.Load(), h.info.Load()} }

func allowUntagged(ctx context.Context, p *redis.Provider) error {
	_, err := p.AllowAll(ctx, []storages.Limit{
		{Key: "host:a", Limit: 100, Period: time.Minute},
		{Key: "global", Limit: 100, Period: time.Minute},
	})
	return err
}

func TestAllowAllVerificationWindow(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")
	hook := &topologyHook{}
	client.AddHook(hook)
	fake := clock.NewFake(time.Time{})
	p := redis.New(client, redis.WithClock(fake))

	for range 8 {
		require.NoError(t, allowUntagged(t.Context(), p))
	}
	require.Equal(t, [2]int64{1, 1}, hook.counts())

	fake.Advance(redis.DefaultTopologyVerificationWindow - time.Nanosecond)
	require.NoError(t, allowUntagged(t.Context(), p))
	require.Equal(t, [2]int64{1, 1}, hook.counts())
	redistest.ServeTopology(mr, "master", "1")
	fake.Advance(time.Nanosecond)
	require.ErrorIs(t, allowUntagged(t.Context(), p), redis.ErrCrossSlot)
	require.Equal(t, [2]int64{2, 2}, hook.counts())

	// Moving the clock back must not revive the superseded check.
	fake.Advance(-time.Nanosecond)
	require.ErrorIs(t, allowUntagged(t.Context(), p), redis.ErrCrossSlot)
	require.Equal(t, [2]int64{3, 3}, hook.counts())
}

func TestAllowAllRetriesFailedVerification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		role string
	}{
		{name: "replica", role: "slave"},
		{name: "ROLE command error"}, // miniredis has no native ROLE
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			if tc.role != "" {
				redistest.ServeTopology(mr, tc.role, "0")
			}
			hook := &topologyHook{}
			client.AddHook(hook)
			p := redis.New(client)
			require.ErrorIs(t, allowUntagged(t.Context(), p), redis.ErrCrossSlot)
			require.Empty(t, mr.Keys())
			require.Equal(t, [2]int64{1, 0}, hook.counts())

			redistest.ServeTopology(mr, "master", "0")
			require.NoError(t, allowUntagged(t.Context(), p))
			require.NoError(t, allowUntagged(t.Context(), p))
			require.Equal(t, [2]int64{2, 1}, hook.counts())
		})
	}
}

func TestAllowAllConcurrentVerification(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")
	hook := &topologyHook{}
	client.AddHook(hook)
	p := redis.New(client)

	const callers = 12
	err := concurrency.Process(t.Context(), make([]struct{}, callers), func(ctx context.Context, _ struct{}) error {
		return allowUntagged(ctx, p)
	}, concurrency.WithConcurrency[struct{}](callers))
	require.NoError(t, err)
	require.Equal(t, [2]int64{1, 1}, hook.counts())
}

func TestAllowAllSharesInFlightFailure(t *testing.T) {
	t.Parallel()
	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		failure := errors.New("ROLE unavailable")
		hook := &topologyHook{onRole: func() error {
			<-release
			return failure
		}}
		client.AddHook(hook)
		p := redis.New(client)
		errs := make(chan error, 2)
		go func() { errs <- allowUntagged(t.Context(), p) }()
		go func() { errs <- allowUntagged(t.Context(), p) }()
		synctest.Wait() // Both callers are blocked before the ROLE reply.
		close(release)
		require.ErrorIs(t, <-errs, failure)
		require.ErrorIs(t, <-errs, failure)
		require.Equal(t, [2]int64{1, 0}, hook.counts())
	})
}

func TestProvider_MultiStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.MultiStorage {
		client, _ := redistest.RedisClient(tb)
		return redis.New(client)
	})
}

// TestProvider_AllowAll_ClusterContract runs the contract through a Cluster
// client, so the keys of every call must share a hash tag and route to one
// node.
func TestProvider_AllowAll_ClusterContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.MultiStorage {
		_, mr := redistest.RedisClient(tb)
		client := goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{mr.Addr()}})
		tb.Cleanup(func() { _ = client.Close() })
		return redis.New(client)
	})
}

// TestProvider_AllowAll_CrossSlot pins the hash-tag check of Cluster and
// Ring clients: keys that hash differently fail with ErrCrossSlot before any
// network I/O (the clients point at a closed port), keys that share a tag
// pass it, and a single-node client accepts any keys.
func TestProvider_AllowAll_CrossSlot(t *testing.T) {
	t.Parallel()
	closed := []string{"127.0.0.1:1"}
	cluster := goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: closed, MaxRetries: -1})
	ring := goredis.NewRing(&goredis.RingOptions{Addrs: map[string]string{"a": closed[0], "b": "127.0.0.1:2"}, MaxRetries: -1})
	t.Cleanup(func() {
		_ = cluster.Close()
		_ = ring.Close()
	})
	single, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")

	tests := []struct {
		name      string
		client    goredis.UniversalClient
		opts      []redis.Option
		keys      []string
		wantIndex int // -1 when the check must pass
	}{
		{name: "cluster untagged", client: cluster, keys: []string{"host:a", "global"}, wantIndex: 1},
		{name: "cluster other tag", client: cluster, keys: []string{"{t}:a", "{t}:b", "{u}:global"}, wantIndex: 2},
		{name: "cluster empty tag hashes whole key", client: cluster, keys: []string{"{}:a", "{}:b"}, wantIndex: 1},
		{name: "coincident hash without shared tag", client: cluster, opts: []redis.Option{redis.WithKeyPrefix("p")}, keys: []string{"t", "{p:t}:other"}, wantIndex: 1},
		{name: "cluster shared tag", client: cluster, keys: []string{"{t}:host:a", "x{t}:global"}, wantIndex: -1},
		{name: "cluster tagged prefix", client: cluster, opts: []redis.Option{redis.WithKeyPrefix("{app}:")}, keys: []string{"a", "b"}, wantIndex: -1},
		{name: "ring untagged", client: ring, keys: []string{"host:a", "global"}, wantIndex: 1},
		{name: "ring shared tag", client: ring, keys: []string{"{t}:host:a", "{t}:global"}, wantIndex: -1},
		{name: "single node", client: single, keys: []string{"host:a", "global"}, wantIndex: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limits := make([]storages.Limit, len(tc.keys))
			for i, k := range tc.keys {
				limits[i] = storages.Limit{Key: k, Limit: 1, Period: time.Minute}
			}
			_, err := redis.New(tc.client, tc.opts...).AllowAll(t.Context(), limits)
			if tc.wantIndex < 0 {
				require.NotErrorIs(t, err, redis.ErrCrossSlot)
				return
			}
			require.ErrorIs(t, err, redis.ErrCrossSlot)
			keyErr, ok := errors.AsType[*storages.KeyError](err)
			require.True(t, ok, "%v", err)
			require.Equal(t, tc.wantIndex, keyErr.Index)
			require.Equal(t, tc.keys[tc.wantIndex], keyErr.Key)
		})
	}
}

// TestProvider_AllowAll_Expiry pins the window TTL of every key of an
// admitted call and the untouched keys of a refused one.
func TestProvider_AllowAll_Expiry(t *testing.T) {
	t.Parallel()
	p, mr := setupProvider(t)
	ctx := t.Context()

	_, err := p.AllowAll(ctx, []storages.Limit{
		{Key: "a", Limit: 1, Period: 2 * time.Second},
		{Key: "b", Limit: 5, Period: 500 * time.Millisecond},
	})
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, mr.TTL(p.Key("a")))
	require.Equal(t, 500*time.Millisecond, mr.TTL(p.Key("b")))

	_, err = p.AllowAll(ctx, []storages.Limit{
		{Key: "c", Limit: 1, Period: time.Minute},
		{Key: "a", Limit: 1, Period: 2 * time.Second},
	})
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
	require.False(t, mr.Exists(p.Key("c")), "a refused call must not create the admitting key")
}

func TestProvider_AllowAll_Panics(t *testing.T) {
	t.Parallel()
	p, _ := setupProvider(t)
	require.Panics(t, func() { _, _ = p.AllowAll(nil, []storages.Limit{{Key: "k", Limit: 1, Period: time.Second}}) }) //nolint:staticcheck // SA1012: nil context under test
	require.Panics(t, func() { _, _ = p.AllowAll(t.Context(), nil) })
	require.Panics(t, func() { _, _ = p.AllowAll(t.Context(), []storages.Limit{{Key: "k", Period: time.Second}}) })
}

func TestAllowAllRejectsUnverifiedClient(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"cluster", "ring", "decorated cluster", "decorated ring", "cluster endpoint", "replica endpoint"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			single, mr := redistest.RedisClient(t)
			var c goredis.UniversalClient
			switch kind {
			case "cluster", "decorated cluster":
				c = goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{mr.Addr()}})
			case "ring", "decorated ring":
				c = goredis.NewRing(&goredis.RingOptions{Addrs: map[string]string{"a": mr.Addr()}, HeartbeatFrequency: time.Hour})
			case "replica endpoint":
				c = single
				redistest.ServeTopology(mr, "slave", "0")
			case "cluster endpoint":
				c = single
				redistest.ServeTopology(mr, "master", "1")
			}
			if c != single {
				t.Cleanup(func() { _ = c.Close() })
			}
			if strings.HasPrefix(kind, "decorated") {
				c = struct{ goredis.UniversalClient }{c}
			}
			_, err := redis.New(c).AllowAll(t.Context(), []storages.Limit{{Key: "a", Limit: 1, Period: time.Minute}, {Key: "b", Limit: 1, Period: time.Minute}})
			require.ErrorIs(t, err, redis.ErrCrossSlot)
			if c != single {
				require.Zero(t, mr.CommandCount())
			}
			require.Empty(t, mr.Keys())
		})
	}
}
