// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
	"github.com/altessa-s/go-atlas/data/cache/storages/lru"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	redisgen "github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
)

var errGeneration = errors.New("generation store down")

// flakyGenerations is a generation store whose calls fail while down is set.
type flakyGenerations struct {
	gen   atomic.Uint64
	down  atomic.Bool
	bumps atomic.Int64
}

func (s *flakyGenerations) Current(context.Context, string) (uint64, error) {
	if s.down.Load() {
		return 0, errGeneration
	}
	return s.gen.Load(), nil
}

func (s *flakyGenerations) Bump(context.Context, string) (uint64, error) {
	if s.down.Load() {
		return 0, errGeneration
	}
	s.bumps.Add(1)
	return s.gen.Add(1), nil
}

// countingStorage counts the writes that reach an LRU storage.
type countingStorage struct {
	*lru.Storage

	saves atomic.Int64
}

func (s *countingStorage) Save(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	s.saves.Add(1)
	return s.Storage.Save(ctx, key, value, ttl)
}

func newLRU(t testing.TB) *countingStorage {
	t.Helper()
	s, err := lru.New(128)
	require.NoError(t, err)
	return &countingStorage{Storage: s}
}

func loadCounter(calls *atomic.Int64, value string) func(context.Context) (item, time.Duration, error) {
	return func(context.Context) (item, time.Duration, error) {
		calls.Add(1)
		return item{Name: value}, cache.TTLUseDefault, nil
	}
}

func TestWithGeneration_InvalidateAllOrphansEveryEntry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	c := cache.New(newLRU(t), cache.WithGeneration(memory.New(), "catalog"))

	require.NoError(t, c.Save(ctx, "a", item{Name: "a"}))
	var calls atomic.Int64
	got, err := cache.GetWithFallbackT(ctx, c, "b", loadCounter(&calls, "b1"))
	require.NoError(t, err)
	require.Equal(t, "b1", got.Name)

	require.NoError(t, c.InvalidateAll(ctx))

	var v item
	require.ErrorIs(t, c.Get(ctx, "a", &v), cache.ErrMissing)
	exists, err := c.Exists(ctx, "a")
	require.NoError(t, err)
	require.False(t, exists)

	got, err = cache.GetWithFallbackT(ctx, c, "b", loadCounter(&calls, "b2"))
	require.NoError(t, err)
	require.Equal(t, "b2", got.Name, "the entry from before the bump is never read")
	require.EqualValues(t, 2, calls.Load())
}

func TestWithGeneration_SharedStoreInvalidatesEveryReplica(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client, _ := testhelpers.RedisClient(t)
	// Zero staleness: the other replica sees the bump on its next read.
	store := redisgen.New(client, redisgen.WithMaxStaleness(0))
	replicaA := cache.New(newLRU(t), cache.WithGeneration(store, "catalog"))
	replicaB := cache.New(newLRU(t), cache.WithGeneration(redisgen.New(client, redisgen.WithMaxStaleness(0)), "catalog"))

	var calls atomic.Int64
	got, err := cache.GetWithFallbackT(ctx, replicaB, "k", loadCounter(&calls, "old"))
	require.NoError(t, err)
	require.Equal(t, "old", got.Name)

	require.NoError(t, replicaA.InvalidateAll(ctx))

	got, err = cache.GetWithFallbackT(ctx, replicaB, "k", loadCounter(&calls, "new"))
	require.NoError(t, err)
	require.Equal(t, "new", got.Name, "a bump on one replica orphans the entries of the other")
}

func TestWithGeneration_FillStartedBeforeBumpIsNeverServed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	c := cache.New(newLRU(t), cache.WithGeneration(memory.New(), "catalog"))

	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var slowErr error
	wg.Go(func() {
		_, slowErr = cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (item, time.Duration, error) {
			close(started)
			<-release
			return item{Name: "pre-write"}, cache.TTLUseDefault, nil
		})
	})
	<-started
	require.NoError(t, c.InvalidateAll(ctx)) // the write commits while the fill runs
	close(release)
	wg.Wait()
	require.NoError(t, slowErr)

	var calls atomic.Int64
	got, err := cache.GetWithFallbackT(ctx, c, "k", loadCounter(&calls, "post-write"))
	require.NoError(t, err)
	require.Equal(t, "post-write", got.Name)
	require.EqualValues(t, 1, calls.Load(), "a reader after the bump never joins nor reads the old fill")
}

func TestWithKeyVersion_KeyLayout(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	storage := newLRU(t)
	c := cache.New(storage,
		cache.WithKeyVersion(func(context.Context) (uint64, error) { return 7, nil }),
		cache.WithKeyNamespace(func(context.Context) string { return "t1" }))

	require.NoError(t, c.Save(ctx, "k", item{Name: "v"}))
	exists, err := storage.Exists(ctx, "7:2:t1:k")
	require.NoError(t, err)
	require.True(t, exists, "version first, then the length-prefixed namespace, then the key")

	require.NoError(t, c.Delete(ctx, "k"))
	exists, err = storage.Exists(ctx, "7:2:t1:k")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestWithKeyVersion_UnknownVersionReadThroughBypassesTheCache(t *testing.T) {
	t.Parallel()

	for _, failOpen := range []bool{false, true} {
		t.Run(map[bool]string{false: "strict", true: "fail-open"}[failOpen], func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			gens := &flakyGenerations{}
			storage := newLRU(t)
			coll := testhelpers.NewTestCollector()
			opts := []cache.Option{cache.WithGeneration(gens, "ns"), cache.WithCollector(coll), cache.WithName("fo")}
			if failOpen {
				opts = append(opts, cache.WithFailOpen(0))
			}
			c := cache.New(storage, opts...)

			gens.down.Store(true)
			var calls atomic.Int64
			for range 2 {
				got, err := cache.GetWithFallbackT(ctx, c, "k", loadCounter(&calls, "fresh"))
				require.NoError(t, err)
				require.Equal(t, "fresh", got.Name)
			}
			var untyped item
			require.NoError(t, c.GetWithFallback(ctx, "k", &untyped, func(context.Context) (any, time.Duration, error) {
				calls.Add(1)
				return &item{Name: "untyped"}, cache.TTLUseDefault, nil
			}))
			require.Equal(t, "untyped", untyped.Name)
			require.EqualValues(t, 3, calls.Load(), "every read goes to the source of truth")
			require.Zero(t, storage.saves.Load(), "nothing is stored under an unknown generation")
			require.InDelta(t, 3, failOpenCount(t, coll, "version"), 0)

			_, err := cache.GetWithFallbackT(ctx, c, "missing", func(context.Context) (*item, time.Duration, error) {
				return nil, cache.TTLUseDefault, nil
			})
			require.ErrorIs(t, err, cache.ErrMissing, "a nil fallback value is a miss on the bypass path too")
		})
	}
}

func TestWithKeyVersion_UnknownVersionOtherOperations(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	strictGens := &flakyGenerations{}
	strict := cache.New(newLRU(t), cache.WithGeneration(strictGens, "ns"))
	strictGens.down.Store(true)

	var v item
	require.ErrorIs(t, strict.Get(ctx, "k", &v), errGeneration)
	require.ErrorIs(t, strict.Save(ctx, "k", item{}), errGeneration)
	_, err := strict.Exists(ctx, "k")
	require.ErrorIs(t, err, errGeneration)
	_, err = strict.GetManyRaw(ctx, []string{"k"})
	require.ErrorIs(t, err, errGeneration)
	require.ErrorIs(t, strict.Delete(ctx, "k"), errGeneration)
	require.ErrorIs(t, strict.DeleteMany(ctx, "k"), errGeneration)
	require.ErrorIs(t, strict.InvalidateAll(ctx), errGeneration)

	openGens := &flakyGenerations{}
	openStorage := newLRU(t)
	open := cache.New(openStorage, cache.WithGeneration(openGens, "ns"), cache.WithFailOpen(0))
	openGens.down.Store(true)

	require.ErrorIs(t, open.Get(ctx, "k", &v), cache.ErrMissing)
	require.NoError(t, open.Save(ctx, "k", item{}))
	require.Zero(t, openStorage.saves.Load(), "a degraded Save stores nothing")
	exists, err := open.Exists(ctx, "k")
	require.NoError(t, err)
	require.False(t, exists)
	raw, err := open.GetManyRaw(ctx, []string{"k"})
	require.NoError(t, err)
	require.Empty(t, raw)
	require.ErrorIs(t, open.Delete(ctx, "k"), errGeneration, "deletes cannot target an unknown generation")
}

func TestInvalidateAll_RequiresGeneration(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	require.ErrorIs(t, cache.New(newLRU(t)).InvalidateAll(ctx), cache.ErrNoGeneration)

	versionOnly := cache.New(newLRU(t),
		cache.WithGeneration(memory.New(), "ns"),
		cache.WithKeyVersion(func(context.Context) (uint64, error) { return 1, nil }))
	require.ErrorIs(t, versionOnly.InvalidateAll(ctx), cache.ErrNoGeneration,
		"WithKeyVersion replaces the generation, InvalidateAll included")
}

func TestWithGeneration_PanicsOnNilStore(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { cache.WithGeneration(nil, "ns") })
}

func TestGetManyT_CleanupTargetsTheEntryThatWasRead(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	storage := newLRU(t)
	// The version changes after the first resolution, as if a bump landed
	// between the bulk read and the cleanup of a corrupt entry.
	var calls atomic.Int64
	version := func(context.Context) (uint64, error) {
		if calls.Add(1) == 1 {
			return 1, nil
		}
		return 2, nil
	}
	c := cache.New(storage, cache.WithKeyVersion(version), cache.WithFailOpen(0))
	require.NoError(t, storage.Storage.Save(ctx, "1:k", []byte("{not json"), 0))
	require.NoError(t, storage.Storage.Save(ctx, "2:k", []byte(`{"Name":"newer"}`), 0))

	got, err := cache.GetManyT[item](ctx, c, []string{"k"})
	require.NoError(t, err)
	require.Empty(t, got, "the corrupt entry is a miss")

	exists, err := storage.Exists(ctx, "1:k")
	require.NoError(t, err)
	require.False(t, exists, "the corrupt entry that was read is deleted")
	exists, err = storage.Exists(ctx, "2:k")
	require.NoError(t, err)
	require.True(t, exists, "the valid entry of the newer version is kept")
	require.EqualValues(t, 1, calls.Load(), "the version is resolved once per bulk call")
}
