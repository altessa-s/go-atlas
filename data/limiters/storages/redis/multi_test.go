// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/redis"
	"github.com/altessa-s/go-atlas/data/limiters/storages/storagetest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	goredis "github.com/redis/go-redis/v9"
)

func TestProvider_MultiStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.MultiStorage {
		client, _ := testhelpers.RedisClient(tb)
		return redis.New(client)
	})
}

// TestProvider_AllowAll_ClusterContract runs the contract through a Cluster
// client, so the keys of every call must share a hash tag and route to one
// node.
func TestProvider_AllowAll_ClusterContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.MultiStorage {
		_, mr := testhelpers.RedisClient(tb)
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
	single, _ := testhelpers.RedisClient(t)

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
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.False(t, mr.Exists(p.Key("c")), "a refused call must not create the admitting key")
}

func TestProvider_AllowAll_Panics(t *testing.T) {
	t.Parallel()
	p, _ := setupProvider(t)
	require.Panics(t, func() { _, _ = p.AllowAll(nil, []storages.Limit{{Key: "k", Limit: 1, Period: time.Second}}) }) //nolint:staticcheck // SA1012: nil context under test
	require.Panics(t, func() { _, _ = p.AllowAll(t.Context(), nil) })
	require.Panics(t, func() { _, _ = p.AllowAll(t.Context(), []storages.Limit{{Key: "k", Period: time.Second}}) })
}
