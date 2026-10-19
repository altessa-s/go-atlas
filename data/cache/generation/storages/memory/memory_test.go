// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/clock"
	"github.com/altessa-s/go-atlas/data/cache/generation"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/storagetest"
)

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func TestStore_Contract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(_ testing.TB, replicas int) storagetest.Backend {
		fake := clock.NewFake(epoch)
		s := memory.New(memory.WithClock(fake))
		return storagetest.Backend{
			Stores:      slices.Repeat([]generation.Store{s}, replicas),
			SetSeedTime: fake.Set,
		}
	}, 0)
}

func TestStore_BumpOfUnseenNamespaceExceedsSeed(t *testing.T) {
	t.Parallel()
	s := memory.New(memory.WithClock(clock.NewFake(epoch)))

	gen, err := s.Bump(t.Context(), "new")
	require.NoError(t, err)
	require.Greater(t, gen, generation.Seed(epoch))
}

func TestStore_BumpFollowsTheClockAndNeverGoesBack(t *testing.T) {
	t.Parallel()
	fake := clock.NewFake(epoch)
	s := memory.New(memory.WithClock(fake))
	ctx := t.Context()

	g0, err := s.Current(ctx, "a")
	require.NoError(t, err)

	fake.Advance(time.Minute)
	g1, err := s.Bump(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, generation.Seed(epoch.Add(time.Minute)), g1, "a bump jumps to the clock when it is ahead")
	require.Greater(t, g1, g0)

	fake.Set(epoch.Add(-time.Hour)) // clock steps back
	g2, err := s.Bump(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, g1+1, g2, "a clock behind the stored value never lowers it")
}

func TestStore_ConcurrentBumpsAreAllCounted(t *testing.T) {
	t.Parallel()
	s := memory.New(memory.WithClock(clock.NewFake(epoch)))
	ctx := t.Context()
	start, err := s.Current(ctx, "a")
	require.NoError(t, err)

	const n = 64
	var wg sync.WaitGroup
	results := make([]uint64, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			results[i], errs[i] = s.Bump(ctx, "a")
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	seen := make(map[uint64]bool, n)
	for _, gen := range results {
		require.False(t, seen[gen], "every bump returns a distinct generation")
		seen[gen] = true
	}
	end, err := s.Current(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, start+n, end)
}
