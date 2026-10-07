// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/auth/denylist/negcache/factory"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// loaderAuth is an authoritative store that is also its own rebuild source,
// like auth/denylist/storages/redis.Store. It counts lookups and streams.
type loaderAuth struct {
	mu      sync.Mutex
	revoked []string
	err     error
	calls   atomic.Int64
	streams atomic.Int64
}

func newLoaderAuth(revoked ...string) *loaderAuth { return &loaderAuth{revoked: revoked} }

func (a *loaderAuth) IsRevoked(_ context.Context, key string) (bool, error) {
	a.calls.Add(1)
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Contains(a.revoked, key), nil
}

func (a *loaderAuth) StreamValues(context.Context) iter.Seq2[string, error] {
	a.streams.Add(1)
	a.mu.Lock()
	keys, err := slices.Clone(a.revoked), a.err
	a.mu.Unlock()
	return func(yield func(string, error) bool) {
		if err != nil {
			yield("", err)
			return
		}
		for _, k := range keys {
			if !yield(k, nil) {
				return
			}
		}
	}
}

func (a *loaderAuth) Count(context.Context) (int64, error) { return -1, nil }

var _ probfilter.DataLoader = (*loaderAuth)(nil)

func isRevoked(t *testing.T, c *negcache.Cache, key string) bool {
	t.Helper()
	got, err := c.IsRevoked(t.Context(), key)
	require.NoError(t, err)
	return got
}

func TestBuild_AuthoritativeLoaderPopulatesOnStart(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	auth := newLoaderAuth("revoked")

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close(context.Background()) })
	require.Equal(t, int64(1), auth.streams.Load(), "rebuildOnStart streams the authoritative store")

	require.False(t, isRevoked(t, cache, "fresh"))
	require.Zero(t, auth.calls.Load(), "a populated cache answers a miss locally")
	require.True(t, isRevoked(t, cache, "revoked"))
}

func TestBuild_RebuildOnStartDisabledLeavesCacheUnpopulated(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	cfg := memoryBloomConfig()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("")
	auth := newLoaderAuth("revoked")

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).Build()
	require.NoError(t, err)
	require.Zero(t, auth.streams.Load())
	require.False(t, isRevoked(t, cache, "fresh"))
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestBuild_UseDataLoaderOverridesAuthoritative(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	auth := newLoaderAuth("revoked")
	explicit := newLoaderAuth("other")

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).
		UseDataLoader(explicit).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close(context.Background()) })
	require.Zero(t, auth.streams.Load())
	require.Equal(t, int64(1), explicit.streams.Load())
}

func TestBuild_NonLoaderAuthoritativeLeavesSettingsInert(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	auth := newLoaderAuth("revoked")
	// Hide the DataLoader methods: only IsRevoked is visible.
	plain := struct{ negcache.Authoritative }{auth}

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, plain).Build()
	require.NoError(t, err)
	require.Zero(t, auth.streams.Load())
	require.False(t, isRevoked(t, cache, "fresh"))
	require.Equal(t, int64(1), auth.calls.Load(), "without a loader the cache stays unpopulated")
}

func TestBuild_LoaderFailureFailsBuild(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	errScan := errors.New("scan failed")
	auth := newLoaderAuth()
	auth.err = errScan

	_, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).Build()
	require.ErrorIs(t, err, errScan)
}

func redisBloomConfig() *probfilterconfig.Filter {
	storage := probfilterconfig.StorageTypeRedis
	return &probfilterconfig.Filter{
		Type: probfilterconfig.TypeBloom,
		Bloom: &probfilterconfig.BloomConfig{
			Storage:       &probfilterconfig.Storage{Type: storage},
			ExpectedItems: 1000,
		},
	}
}

func TestBuild_RedisFilterRegistersSchedulerTask(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	defaults := probfilterconfig.NewDefaults()
	cfg := redisBloomConfig()
	cfg.Bloom.RebuildOnStart = new(false) // miniredis cannot run RedisBloom commands
	cfg.Bloom.RebuildCron = new("@every 5m")
	sched := &testhelpers.MockTaskRegistrar{}

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).
		UseRedisClient(client).
		UseScheduler(sched).
		Build()
	require.NoError(t, err)

	task, ok := sched.Task("probfilter-rebuild-denylist")
	require.True(t, ok)
	require.Equal(t, "@every 5m", task.Schedule)

	require.NoError(t, cache.Close(t.Context()))
	require.NoError(t, task.Func(t.Context()), "the closed cache's task is a no-op")
}

// leaseHolder streams nothing until released, keeping a peer's rebuild
// lease held.
type leaseHolder struct {
	started chan struct{}
	release chan struct{}
}

func (l *leaseHolder) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		close(l.started)
		<-l.release
		yield("peer", nil)
	}
}

func (l *leaseHolder) Count(context.Context) (int64, error) { return -1, nil }

func TestBuild_PeerRebuildingSharedFilterBuildsUnpopulated(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	defaults := probfilterconfig.NewDefaults()
	cfg := redisBloomConfig()
	cfg.Bloom.RebuildCron = new("")

	holdLease(t, client, "denylist")

	auth := newLoaderAuth("revoked")
	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).
		UseRedisClient(client).
		Build()
	require.NoError(t, err, "a peer's rebuild in progress must not fail Build")
	t.Cleanup(func() { _ = cache.Close(context.Background()) })
	require.Zero(t, auth.streams.Load(), "the refused rebuild did not read its source")

	// Unpopulated: lookups go to the authoritative store (the shared filter
	// has no committed rebuild).
	require.True(t, isRevoked(t, cache, "revoked"))
	require.Equal(t, int64(1), auth.calls.Load())
}

// holdLease starts a rebuild of the shared filter name by a peer process that
// holds the rebuild lease until the test ends.
func holdLease(t *testing.T, client goredis.UniversalClient, name string) {
	t.Helper()
	peer := bloom.New(bloomredis.New(client, name))
	loader := &leaseHolder{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = peer.Rebuild(context.Background(), loader) // miniredis cannot stage; the error is irrelevant
	}()
	<-loader.started
	t.Cleanup(func() {
		close(loader.release)
		<-done
	})
}

func TestCache_CloseStopsLocalCron(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	cfg := memoryBloomConfig()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("@every 1s")
	auth := newLoaderAuth()

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).Build()
	require.NoError(t, err)
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return auth.streams.Load() >= 1 }, "the local cron never rebuilt")

	require.NoError(t, cache.Close(t.Context()))
	streams := auth.streams.Load()
	time.Sleep(1500 * time.Millisecond) // longer than the cron interval
	require.Equal(t, streams, auth.streams.Load(), "a closed cache's filter is not rebuilt again")
}
