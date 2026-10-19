// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru_test

import (
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/clock"
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

	fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	cache := lru.NewExpirableCache[string, int](10, time.Minute, lru.WithExpirableClock(fake.Now))
	cache.Put("a", 1)

	// Read continuously, every 10 seconds of the fake clock, up to just before the TTL.
	for range 5 {
		fake.Advance(10 * time.Second)
		_, ok := cache.Get("a")
		require.True(t, ok)
	}
	fake.Advance(10*time.Second - time.Nanosecond)
	_, ok := cache.Get("a")
	require.True(t, ok, "alive one nanosecond before its TTL")

	fake.Advance(time.Nanosecond)
	_, ok = cache.Get("a")
	require.False(t, ok, "expired at its TTL despite continuous reads")
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

	pairs := maps.Collect(cache.All())
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

func TestExpirableCache_PutIfAbsent(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)

	actual, loaded := cache.PutIfAbsent("a", 1)
	require.False(t, loaded)
	require.Equal(t, 1, actual)

	actual, loaded = cache.PutIfAbsent("a", 2)
	require.True(t, loaded)
	require.Equal(t, 1, actual, "an existing entry is kept")
	got, _ := cache.Get("a")
	require.Equal(t, 1, got)
}

func TestExpirableCache_PutIfAbsentAfterExpiry(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, 20*time.Millisecond)
	_, loaded := cache.PutIfAbsent("a", 1)
	require.False(t, loaded)
	require.Eventually(t, func() bool { return !cache.Has("a") }, time.Second, 5*time.Millisecond)

	actual, loaded := cache.PutIfAbsent("a", 2)
	require.False(t, loaded, "an expired entry counts as absent")
	require.Equal(t, 2, actual)
}

func TestExpirableCache_CompareAndSwap(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)
	require.False(t, cache.CompareAndSwap("a", 0, 1), "a missing key never matches, not even the zero value")
	_, ok := cache.Get("a")
	require.False(t, ok)

	cache.Put("a", 1)
	require.False(t, cache.CompareAndSwap("a", 2, 3))
	got, _ := cache.Get("a")
	require.Equal(t, 1, got)

	require.True(t, cache.CompareAndSwap("a", 1, 3))
	got, _ = cache.Get("a")
	require.Equal(t, 3, got)
}

func TestExpirableCache_CompareAndSwapNonComparablePanics(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, []int](10, time.Minute)
	cache.Put("a", []int{1})
	require.Panics(t, func() { cache.CompareAndSwap("a", []int{1}, []int{2}) })
}

// TestExpirableCache_ConditionalWritesAreAtomic races claimers the way a once-per-cooldown gate does: exactly one PutIfAbsent wins
// a fresh key, and of the claimers that saw the same value exactly one CompareAndSwap wins.
func TestExpirableCache_ConditionalWritesAreAtomic(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpirableCache[string, int](10, time.Minute)
	const workers = 64

	var wins sync.WaitGroup
	won := make(chan int, workers)
	for i := range workers {
		wins.Go(func() {
			if _, loaded := cache.PutIfAbsent("k", i); !loaded {
				won <- i
			}
		})
	}
	wins.Wait()
	close(won)
	require.Len(t, won, 1)
	first := <-won

	swapped := make(chan int, workers)
	var swaps sync.WaitGroup
	for i := range workers {
		swaps.Go(func() {
			if cache.CompareAndSwap("k", first, 1000+i) {
				swapped <- i
			}
		})
	}
	swaps.Wait()
	close(swapped)
	require.Len(t, swapped, 1)
}

// TestExpirableCache_ExpiryIsAtomicWithConditionalWrites drives expiry with an injected clock: from the instant an entry expires,
// every reader and writer sees it absent — CompareAndSwap never revives it, PutIfAbsent stores afresh — whatever the background
// reaper has done.
func TestExpirableCache_ExpiryIsAtomicWithConditionalWrites(t *testing.T) {
	t.Parallel()

	fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	cache := lru.NewExpirableCache[string, int](10, time.Minute, lru.WithExpirableClock(fake.Now))

	cache.Put("k", 1)
	fake.Advance(time.Minute - time.Nanosecond)
	got, ok := cache.Get("k")
	require.True(t, ok)
	require.Equal(t, 1, got)
	require.True(t, cache.CompareAndSwap("k", 1, 2), "alive one nanosecond before expiry")

	// The swap restarted the TTL: the entry now expires a minute after the swap.
	fake.Advance(time.Minute)
	_, ok = cache.Get("k")
	require.False(t, ok, "expired exactly at its expiry instant")
	require.False(t, cache.Has("k"))
	require.False(t, cache.CompareAndSwap("k", 2, 3), "an entry readers see as expired is never replaced")
	_, ok = cache.Get("k")
	require.False(t, ok, "and stays absent")
	require.Empty(t, slices.Collect(cache.Keys()))

	actual, loaded := cache.PutIfAbsent("k", 4)
	require.False(t, loaded, "an expired entry counts as absent")
	require.Equal(t, 4, actual)
	got, ok = cache.Get("k")
	require.True(t, ok)
	require.Equal(t, 4, got)
}

func TestExpirableCache_ZeroTTLNeverExpires(t *testing.T) {
	t.Parallel()

	fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	cache := lru.NewExpirableCache[string, int](10, 0, lru.WithExpirableClock(fake.Now))
	cache.Put("k", 1)
	fake.Advance(24 * 365 * time.Hour)
	require.True(t, cache.CompareAndSwap("k", 1, 2))
	got, ok := cache.Get("k")
	require.True(t, ok)
	require.Equal(t, 2, got)
}

// TestExpirableCache_OneClock: with an injected clock, real time alone never expires an entry — expiry and cleanup follow the same
// clock — and a write sweeps the entries that clock has expired.
func TestExpirableCache_OneClock(t *testing.T) {
	t.Parallel()

	fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	cache := lru.NewExpirableCache[string, int](10, 20*time.Millisecond, lru.WithExpirableClock(fake.Now))
	cache.Put("a", 1)
	cache.Put("b", 2)

	time.Sleep(60 * time.Millisecond) // three real TTLs; the fake clock stands still
	got, ok := cache.Get("a")
	require.True(t, ok, "real elapsed time must not expire an entry")
	require.Equal(t, 1, got)
	require.True(t, cache.CompareAndSwap("b", 2, 3))
	require.Equal(t, 2, cache.Len())

	fake.Advance(20 * time.Millisecond)
	require.False(t, cache.Has("a"))
	require.Equal(t, 2, cache.Len(), "expired entries wait for a write")
	cache.Put("c", 4)
	require.Equal(t, 1, cache.Len(), "the write swept the expired entries")
	require.Equal(t, []string{"c"}, slices.Collect(cache.Keys()))
}

// TestExpirableCache_ExpiredEntriesDoNotDisplaceLiveOnes: reading an expired entry does not refresh its recency, so on eviction it
// goes before live entries.
func TestExpirableCache_ExpiredEntriesDoNotDisplaceLiveOnes(t *testing.T) {
	t.Parallel()

	fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	cache := lru.NewExpirableCache[string, int](2, time.Minute, lru.WithExpirableClock(fake.Now))
	cache.Put("old", 1)
	fake.Advance(30 * time.Second)
	cache.Put("live", 2)
	fake.Advance(30 * time.Second) // "old" expires, "live" has 30s left
	_, ok := cache.Get("old")
	require.False(t, ok)
	cache.Put("new", 3) // the sweep (or eviction) drops "old", never "live"
	require.True(t, cache.Has("live"))
	require.True(t, cache.Has("new"))
}

// TestExpirableCache_StaggeredExpiryKeepsLiveEntries is the review scenario: an expired but recently read entry must not survive
// while a live one is evicted.
func TestExpirableCache_StaggeredExpiryKeepsLiveEntries(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	fake := clock.NewFake(t0)
	at := func(sec int) { fake.Set(t0.Add(time.Duration(sec) * time.Second)) }
	cache := lru.NewExpirableCache[string, int](2, 10*time.Second, lru.WithExpirableClock(fake.Now))

	cache.Put("A", 1) // t=0
	at(5)
	cache.Put("A", 2) // refresh: A expires at 15
	at(6)
	cache.Put("B", 3)
	at(10)
	cache.Put("B", 4) // refresh: B expires at 20; the periodic sweep runs here and finds nothing
	at(14)
	got, ok := cache.Get("A") // A is now the most recently used
	require.True(t, ok)
	require.Equal(t, 2, got)
	at(16)
	cache.Put("C", 5) // A expired at 15: it must go, not B

	got, ok = cache.Get("B")
	require.True(t, ok, "a live entry was evicted while an expired one remained")
	require.Equal(t, 4, got)
	require.True(t, cache.Has("C"))
	require.False(t, cache.Has("A"))
}

// expirableModel is the reference: an LRU over live entries that drops every expired entry before evicting a live one.
type expirableModel struct {
	cap     int
	ttl     time.Duration
	entries map[string]*modelEntry
	tick    int
}

type modelEntry struct {
	value     int
	expiresAt time.Time
	used      int
}

func (m *expirableModel) live(k string, now time.Time) (*modelEntry, bool) {
	e, ok := m.entries[k]
	if !ok || !now.Before(e.expiresAt) {
		return nil, false
	}
	return e, true
}

func (m *expirableModel) touch(e *modelEntry) { m.tick++; e.used = m.tick }

func (m *expirableModel) store(k string, v int, now time.Time) {
	if _, present := m.entries[k]; !present && len(m.entries) >= m.cap {
		for key, e := range m.entries {
			if !now.Before(e.expiresAt) {
				delete(m.entries, key)
			}
		}
		if len(m.entries) >= m.cap {
			victim, oldest := "", 0
			for key, e := range m.entries {
				if victim == "" || e.used < oldest {
					victim, oldest = key, e.used
				}
			}
			delete(m.entries, victim)
		}
	}
	e := &modelEntry{value: v, expiresAt: now.Add(m.ttl)}
	m.touch(e)
	m.entries[k] = e
}

// TestExpirableCache_MatchesReferenceModel runs random operation sequences with a fake clock and checks after every step that the
// visible state equals the reference model's.
func TestExpirableCache_MatchesReferenceModel(t *testing.T) {
	t.Parallel()

	keys := []string{"a", "b", "c", "d", "e"}
	for seed := range uint64(20) {
		rng := rand.New(rand.NewPCG(seed, seed*7+1))
		fake := clock.NewFake(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
		cache := lru.NewExpirableCache[string, int](3, 10*time.Second, lru.WithExpirableClock(fake.Now))
		model := &expirableModel{cap: 3, ttl: 10 * time.Second, entries: map[string]*modelEntry{}}

		for step := range 2000 {
			k := keys[rng.IntN(len(keys))]
			v := rng.IntN(1000)
			now := fake.Now()
			switch op := rng.IntN(5); op {
			case 0:
				cache.Put(k, v)
				model.store(k, v, now)
			case 1:
				got, ok := cache.Get(k)
				e, live := model.live(k, now)
				require.Equal(t, live, ok, "seed %d step %d: Get(%s)", seed, step, k)
				if live {
					require.Equal(t, e.value, got)
					model.touch(e)
				}
			case 2:
				actual, loaded := cache.PutIfAbsent(k, v)
				if e, live := model.live(k, now); live {
					require.True(t, loaded, "seed %d step %d: PutIfAbsent(%s)", seed, step, k)
					require.Equal(t, e.value, actual)
					model.touch(e)
				} else {
					require.False(t, loaded, "seed %d step %d: PutIfAbsent(%s)", seed, step, k)
					model.store(k, v, now)
				}
			case 3:
				old := v % 3
				swapped := cache.CompareAndSwap(k, old, v)
				e, live := model.live(k, now)
				want := live && e.value == old
				require.Equal(t, want, swapped, "seed %d step %d: CompareAndSwap(%s)", seed, step, k)
				if want {
					model.store(k, v, now)
				}
			default:
				fake.Advance(time.Duration(rng.IntN(7)) * time.Second)
			}
			now = fake.Now()
			for _, key := range keys {
				_, live := model.live(key, now)
				require.Equal(t, live, cache.Has(key), "seed %d step %d: Has(%s)", seed, step, key)
			}
		}
	}
}
