// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"context"
	"iter"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"

	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// newRedisBloomIT connects to a live Redis with the RedisBloom module
// (REDIS_ADDR, default localhost:6379) and returns a client plus a per-test
// key prefix whose keys are dropped on cleanup. It skips when Redis or the
// module is unavailable.
func newRedisBloomIT(t *testing.T) (*goredis.Client, string) {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	prefix := "negcache_it:" + strconv.FormatInt(time.Now().UnixNano(), 10) + ":"
	if err := client.Do(t.Context(), "BF.RESERVE", prefix+"probe", 0.01, 10).Err(); err != nil {
		t.Skipf("RedisBloom module not available at %s: %v", addr, err)
	}
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		keys, _ := client.Keys(ctx, "*"+prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
	})
	return client, prefix
}

// pausedLoader streams the revoked set of an authoritative store once
// resumed, signaling when the rebuild holding the lease started reading it.
type pausedLoader struct {
	auth    *syncAuth
	started chan struct{}
	resume  chan struct{}
}

func (l *pausedLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	close(l.started)
	<-l.resume
	return l.auth.StreamValues(ctx)
}

func (l *pausedLoader) Count(context.Context) (int64, error) { return -1, nil }

// TestCache_SharedRedisFilterPopulatedByPeer_Integration runs two nodes over
// one Redis Bloom filter: node B's rebuild is refused while node A rebuilds,
// so B never rebuilds itself, yet it trusts the shared filter as soon as A's
// rebuild is committed — and never answers a revoked key "not revoked".
func TestCache_SharedRedisFilterPopulatedByPeer_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t)
	ctx := t.Context()
	auth := &syncAuth{}
	auth.revoke("revoked")

	newNode := func() *negcache.Cache {
		filter := bloom.New(bloomredis.New(client, "denylist", bloomredis.WithKeyPrefix(prefix), bloomredis.WithExpectedItems(1000)))
		return negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(time.Nanosecond))
	}
	nodeA, nodeB := newNode(), newNode()

	loader := &pausedLoader{auth: auth, started: make(chan struct{}), resume: make(chan struct{})}
	doneA := make(chan error, 1)
	go func() { doneA <- nodeA.Rebuild(ctx, loader) }()
	<-loader.started

	require.ErrorIs(t, nodeB.Rebuild(ctx, auth), probfilter.ErrRebuildInProgress)
	calls := auth.calls.Load()
	got, err := nodeB.IsRevoked(ctx, "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, calls+1, auth.calls.Load(), "before any commit node B defers to the authoritative store")

	close(loader.resume)
	require.NoError(t, <-doneA)

	calls = auth.calls.Load()
	got, err = nodeB.IsRevoked(ctx, "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, calls, auth.calls.Load(), "node B trusts the shared filter once node A committed")

	got, err = nodeB.IsRevoked(ctx, "revoked")
	require.NoError(t, err)
	require.True(t, got, "a revoked key is never fast-pathed")

	require.NoError(t, nodeA.Close(ctx))
	require.NoError(t, nodeB.Close(ctx))
}
