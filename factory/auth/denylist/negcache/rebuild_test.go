// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/probfiltertest"
	"github.com/altessa-s/go-atlas/factory/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
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

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).Build(t.Context())
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

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).Build(t.Context())
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
		Build(t.Context())
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

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, plain).Build(t.Context())
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

	_, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).Build(t.Context())
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
	defaults := probfilterconfig.NewDefaults()
	cfg := redisBloomConfig()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("@every 5m")
	sched := &testhelpers.MockTaskRegistrar{}

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)

	task, ok := sched.Task("probfilter-rebuild-denylist")
	require.True(t, ok)
	require.Equal(t, "@every 5m", task.Schedule)

	require.NoError(t, cache.Close(t.Context()))
	require.NoError(t, task.Func(t.Context()), "the closed cache's task is a no-op")
}

func TestBuild_PeerRebuildingSharedFilterBuildsUnpopulated(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	cfg := redisBloomConfig()
	cfg.Bloom.RebuildCron = new("")

	auth := newLoaderAuth("revoked")
	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(true)).
		Build(t.Context())
	require.NoError(t, err, "a peer's rebuild in progress must not fail Build")
	t.Cleanup(func() { _ = cache.Close(context.Background()) })
	require.Zero(t, auth.streams.Load(), "the refused rebuild did not read its source")

	// Unpopulated: lookups go to the authoritative store (the shared filter
	// has no committed rebuild).
	require.True(t, isRevoked(t, cache, "revoked"))
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestCache_CloseStopsLocalCron(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	cfg := memoryBloomConfig()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("@every 1s")
	auth := newLoaderAuth()

	cache, err := factory.NewBuilder("denylist", cfg, &defaults, auth).Build(t.Context())
	require.NoError(t, err)
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return auth.streams.Load() >= 1 }, "the local cron never rebuilt")

	require.NoError(t, cache.Close(t.Context()))
	streams := auth.streams.Load()
	time.Sleep(1500 * time.Millisecond) // longer than the cron interval
	require.Equal(t, streams, auth.streams.Load(), "a closed cache's filter is not rebuilt again")
}

// warnRecorder is a slog.Handler that keeps the messages logged at Warn level.
type warnRecorder struct {
	mu    sync.Mutex
	warns []string
}

func (r *warnRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *warnRecorder) Handle(_ context.Context, rec slog.Record) error {
	if rec.Level == slog.LevelWarn {
		r.mu.Lock()
		r.warns = append(r.warns, rec.Message)
		r.mu.Unlock()
	}
	return nil
}

func (r *warnRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *warnRecorder) WithGroup(string) slog.Handler      { return r }

// unscheduledWarnings counts the "no periodic filter rebuild" warnings.
func (r *warnRecorder) unscheduledWarnings() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, msg := range r.warns {
		if strings.HasPrefix(msg, "negcache: no periodic filter rebuild is scheduled") {
			n++
		}
	}
	return n
}

func TestBuild_WarnsWhenNoRebuildIsScheduled(t *testing.T) {
	t.Parallel()

	noCron := func(cfg *probfilterconfig.Filter) *probfilterconfig.Filter {
		cfg.Bloom.RebuildOnStart = new(false)
		cfg.Bloom.RebuildCron = new("")
		return cfg
	}
	redisNoStart := func() *probfilterconfig.Filter {
		cfg := redisBloomConfig()
		cfg.Bloom.RebuildOnStart = new(false)
		return cfg
	}
	cuckoo := &probfilterconfig.Filter{
		Type:   probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{Capacity: 1000},
	}

	tests := []struct {
		name      string
		cfg       *probfilterconfig.Filter
		auth      negcache.Authoritative
		scheduler bool
		wantWarn  int
	}{
		{name: "default cron with a loader is scheduled", cfg: memoryBloomConfig(), auth: newLoaderAuth()},
		{name: "no data loader", cfg: memoryBloomConfig(), auth: newFakeAuth(), wantWarn: 1},
		{name: "empty rebuildCron", cfg: noCron(memoryBloomConfig()), auth: newLoaderAuth(), wantWarn: 1},
		{name: "cuckoo filter", cfg: cuckoo, auth: newLoaderAuth(), wantWarn: 1},
		{name: "shared filter without a scheduler", cfg: redisNoStart(), auth: newLoaderAuth(), wantWarn: 1},
		{name: "shared filter with a scheduler", cfg: redisNoStart(), auth: newLoaderAuth(), scheduler: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defaults := probfilterconfig.NewDefaults()
			rec := &warnRecorder{}
			b := factory.NewBuilder("denylist", tc.cfg, &defaults, tc.auth).
				UseLogger(slog.New(rec)).
				UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false))
			if tc.scheduler {
				b.UseScheduler(&testhelpers.MockTaskRegistrar{})
			}

			cache, err := b.Build(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = cache.Close(context.Background()) })
			require.Equal(t, tc.wantWarn, rec.unscheduledWarnings())
		})
	}
}

var errBuild = errors.New("storage build failed")

func TestBuild_RedisStorageRegistration(t *testing.T) {
	t.Parallel()
	defaults := probfilterconfig.NewDefaults()
	cfg := redisBloomConfig()

	_, err := factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).Build(t.Context())
	require.Error(t, err, "redis not registered")

	_, err = factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, nil).
		Build(t.Context())
	require.Error(t, err)

	_, err = factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, nil).
		Build(t.Context())
	require.Error(t, err)

	_, err = factory.NewBuilder("denylist", cfg, &defaults, newLoaderAuth()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis,
			func(context.Context, probfilterfactory.BloomStorageSpec) (bloomstorages.Storage, error) {
				return nil, errBuild
			}).
		Build(t.Context())
	require.ErrorIs(t, err, errBuild)
}

// sharedBloom returns a storage function building a shared in-memory storage,
// for registration as the redis type.
func sharedBloom(peerRebuilding bool) probfilterfactory.BloomStorageFunc {
	return func(context.Context, probfilterfactory.BloomStorageSpec) (bloomstorages.Storage, error) {
		return probfiltertest.NewSharedBloom(peerRebuilding), nil
	}
}
