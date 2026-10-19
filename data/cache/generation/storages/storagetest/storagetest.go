// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/generation"
)

// Backend is one empty generation backend under test, isolated from every
// other backend the harness returns.
type Backend struct {
	// Stores are the replicas sharing the backend, as many as requested. An
	// in-process backend returns the same store repeated.
	Stores []generation.Store
	// SetSeedTime sets the clock the backend seeds fresh namespaces from (the
	// Redis server clock for storages/redis). Nil when that clock cannot be
	// controlled, such as a live server: the contracts that need it skip.
	SetSeedTime func(time.Time)
	// Advance moves the clock that measures the replicas' local staleness
	// forward. Required when the staleness passed to [Run] is positive.
	Advance func(time.Duration)
}

// NewBackend returns a fresh [Backend] with the given number of replicas (at
// least one).
type NewBackend func(tb testing.TB, replicas int) Backend

// contracts lists the store contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, NewBackend, time.Duration)
}{
	{"SeedFromClock", SeedFromClock},
	{"BumpVisibleAtOnce", BumpVisibleAtOnce},
	{"BumpVisibleWithinBound", BumpVisibleWithinBound},
	{"Monotonic", Monotonic},
	{"ConcurrentBumps", ConcurrentBumps},
	{"NamespaceIsolation", NamespaceIsolation},
}

// epoch is the fixtures' reference instant for controllable seed clocks.
var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// pair is the replica count of the contracts comparing a writer and a reader.
const pair = 2

// concurrency is the number of replicas bumping at once in [ConcurrentBumps].
const concurrency = 8

// Run executes every contract as a parallel subtest named after it, each on a
// fresh backend from newBackend. staleness is the visibility bound of the
// backend: a Current call starting strictly later than staleness after a bump
// returned, as measured by [Backend.Advance], sees it on every replica. Zero
// means at once (an in-process store).
func Run(t *testing.T, newBackend NewBackend, staleness time.Duration) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.check(t, newBackend, staleness)
		})
	}
}

// SeedFromClock verifies that a fresh namespace starts at [generation.Seed]
// of the seed clock, keeps that generation when the clock moves on, and is
// seen with the same seed by every replica.
func SeedFromClock(t *testing.T, newBackend NewBackend, _ time.Duration) {
	b := newBackend(t, pair)
	if b.SetSeedTime == nil {
		t.Skip("the seed clock of this backend cannot be controlled")
	}
	ctx := t.Context()

	b.SetSeedTime(epoch)
	gen, err := b.Stores[0].Current(ctx, "seeded")
	require.NoError(t, err)
	require.Equal(t, generation.Seed(epoch), gen, "a fresh namespace is seeded from the clock, not zero")

	later := epoch.Add(time.Hour)
	b.SetSeedTime(later)
	again, err := b.Stores[0].Current(ctx, "seeded")
	require.NoError(t, err)
	require.Equal(t, gen, again, "a seeded namespace keeps its generation")

	shared, err := b.Stores[1].Current(ctx, "seeded")
	require.NoError(t, err)
	require.Equal(t, gen, shared, "every replica sees the same seed")

	fresh, err := b.Stores[1].Current(ctx, "fresh")
	require.NoError(t, err)
	require.Equal(t, generation.Seed(later), fresh, "each namespace is seeded on first use")
}

// BumpVisibleAtOnce verifies that a bump exceeds the previous generation and
// is returned by the next Current of the store that bumped, with no clock
// advance.
func BumpVisibleAtOnce(t *testing.T, newBackend NewBackend, _ time.Duration) {
	s := newBackend(t, 1).Stores[0]
	ctx := t.Context()

	g0, err := s.Current(ctx, "ns")
	require.NoError(t, err)
	g1, err := s.Bump(ctx, "ns")
	require.NoError(t, err)
	require.Greater(t, g1, g0)

	got, err := s.Current(ctx, "ns")
	require.NoError(t, err)
	require.Equal(t, g1, got, "no staleness on the store that bumped")
}

// BumpVisibleWithinBound verifies that a bump on one replica is observed by
// another replica, which read the namespace before, once the staleness bound
// has passed.
func BumpVisibleWithinBound(t *testing.T, newBackend NewBackend, staleness time.Duration) {
	b := newBackend(t, pair)
	writer, reader := b.Stores[0], b.Stores[1]
	ctx := t.Context()

	g0, err := reader.Current(ctx, "ns")
	require.NoError(t, err)
	g1, err := writer.Bump(ctx, "ns")
	require.NoError(t, err)
	require.Greater(t, g1, g0)

	settle(t, b, staleness)
	got, err := reader.Current(ctx, "ns")
	require.NoError(t, err)
	require.GreaterOrEqual(t, got, g1, "once the bound has passed every replica sees the bump")
}

// Monotonic verifies that interleaved Current and Bump calls of one store
// never return a smaller generation, that every bump exceeds everything
// returned before it, and that a seed clock stepping back changes neither.
func Monotonic(t *testing.T, newBackend NewBackend, staleness time.Duration) {
	b := newBackend(t, 1)
	s := b.Stores[0]
	ctx := t.Context()

	var last uint64
	for i := range 12 {
		if b.SetSeedTime != nil && i%4 == 2 {
			b.SetSeedTime(epoch.Add(-time.Duration(i) * time.Hour)) // the seed clock steps back
		}
		if i%2 == 0 {
			gen, err := s.Bump(ctx, "ns")
			require.NoError(t, err)
			require.Greater(t, gen, last, "a bump exceeds every generation returned before it")
			last = gen
		} else {
			gen, err := s.Current(ctx, "ns")
			require.NoError(t, err)
			require.GreaterOrEqual(t, gen, last, "a generation never goes backwards")
			last = gen
		}
		settle(t, b, staleness)
	}
}

// ConcurrentBumps verifies that bumps racing on several replicas all succeed
// with distinct generations above the starting one, and that every replica
// converges on the highest once the bound has passed.
func ConcurrentBumps(t *testing.T, newBackend NewBackend, staleness time.Duration) {
	b := newBackend(t, concurrency)
	ctx := t.Context()
	start, err := b.Stores[0].Current(ctx, "ns")
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]uint64, concurrency)
	errs := make([]error, concurrency)
	for i, s := range b.Stores {
		wg.Go(func() { results[i], errs[i] = s.Bump(ctx, "ns") })
	}
	wg.Wait()

	seen := make(map[uint64]bool, concurrency)
	for i, gen := range results {
		require.NoError(t, errs[i])
		require.Greater(t, gen, start)
		require.False(t, seen[gen], "every bump returns a distinct generation")
		seen[gen] = true
	}

	settle(t, b, staleness)
	top := slices.Max(results)
	for _, s := range b.Stores {
		got, err := s.Current(ctx, "ns")
		require.NoError(t, err)
		require.Equal(t, top, got, "every replica converges on the last bump")
	}
}

// NamespaceIsolation verifies that a bump of one namespace leaves the
// generation of another unchanged on every replica.
func NamespaceIsolation(t *testing.T, newBackend NewBackend, staleness time.Duration) {
	b := newBackend(t, pair)
	ctx := t.Context()

	a0, err := b.Stores[0].Current(ctx, "a")
	require.NoError(t, err)
	b0, err := b.Stores[0].Current(ctx, "b")
	require.NoError(t, err)

	_, err = b.Stores[0].Bump(ctx, "a")
	require.NoError(t, err)
	got, err := b.Stores[0].Current(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, b0, got, "a bump of one namespace leaves the others")

	settle(t, b, staleness)
	got, err = b.Stores[1].Current(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, b0, got, "on every replica")
	got, err = b.Stores[1].Current(ctx, "a")
	require.NoError(t, err)
	require.Greater(t, got, a0)
}

// settle advances the staleness clock of b strictly past the bound, so every
// replica observes the backend again.
func settle(tb testing.TB, b Backend, staleness time.Duration) {
	tb.Helper()
	if staleness <= 0 {
		return
	}
	require.NotNil(tb, b.Advance, "a backend with a staleness bound must provide Advance")
	b.Advance(staleness + time.Nanosecond)
}
