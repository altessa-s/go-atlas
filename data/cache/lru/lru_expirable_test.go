// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/lru"
)

func TestExpirableCache_GetPutRemove(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)

	_, ok := cache.Get("missing")
	require.False(t, ok)

	require.False(t, cache.Put("a", 1))

	got, ok := cache.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, got)
	require.True(t, cache.Has("a"))
	require.Equal(t, 1, cache.Len())

	require.True(t, cache.Remove("a"))
	require.False(t, cache.Remove("a"))
	require.False(t, cache.Has("a"))
}

// The property that distinguishes this from a plain LRU: an entry stops being
// served once it ages out, even when the cache is nowhere near full.
func TestExpirableCache_EntriesExpire(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, 20*time.Millisecond)
	cache.Put("a", 1)

	got, ok := cache.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, got)

	require.Eventually(t, func() bool {
		_, ok := cache.Get("a")
		return !ok
	}, time.Second, 5*time.Millisecond, "entry must stop being served after its TTL")
}

// Reads must not extend the lifetime, otherwise a hot key never refreshes and
// the TTL stops bounding staleness.
func TestExpirableCache_ReadsDoNotExtendTTL(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, 60*time.Millisecond)
	cache.Put("a", 1)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := cache.Get("a"); !ok {
			return // expired despite continuous reads, as intended
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("entry survived well past its TTL under continuous reads")
}

func TestExpirableCache_EvictsWhenFull(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](2, time.Minute)

	cache.Put("a", 1)
	cache.Put("b", 2)
	require.True(t, cache.Put("c", 3), "adding past capacity must evict")

	require.Equal(t, 2, cache.Len())
}

func TestExpirableCache_Purge(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)
	cache.Put("a", 1)
	cache.Put("b", 2)

	cache.Purge()
	require.Zero(t, cache.Len())
}

func TestExpirableCache_Iterators(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)
	cache.Put("a", 1)
	cache.Put("b", 2)

	keys := slices.Collect(cache.Keys())
	slices.Sort(keys)
	require.Equal(t, []string{"a", "b"}, keys)

	pairs := map[string]int{}
	for k, v := range cache.All() {
		pairs[k] = v
	}
	require.Equal(t, map[string]int{"a": 1, "b": 2}, pairs)
}

func TestExpirableCache_GetOrCompute(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)

	var calls int
	compute := func(context.Context) (int, error) {
		calls++
		return 42, nil
	}

	got, err := cache.GetOrCompute(t.Context(), "a", compute)
	require.NoError(t, err)
	require.Equal(t, 42, got)

	got, err = cache.GetOrCompute(t.Context(), "a", compute)
	require.NoError(t, err)
	require.Equal(t, 42, got)
	require.Equal(t, 1, calls, "second call must be served from the cache")
}

func TestExpirableCache_GetOrComputeError(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)
	sentinel := errors.New("boom")

	_, err := cache.GetOrCompute(t.Context(), "a", func(context.Context) (int, error) {
		return 0, sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, cache.Len(), "a failed computation must not be cached")
}

// Concurrent misses on the same key must collapse into one computation.
func TestExpirableCache_GetOrComputeSingleflight(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)

	var (
		mu    sync.Mutex
		calls int
	)

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			_, err := cache.GetOrCompute(t.Context(), "a", func(context.Context) (int, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)

				return 7, nil
			})
			require.NoError(t, err)
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls)
}

func TestExpirableCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[int, int](64, time.Minute)

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			cache.Put(i, i)
			cache.Get(i)
			cache.Has(i)
			cache.Len()
		})
	}
	wg.Wait()
}
