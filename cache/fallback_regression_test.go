// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages/freecache"
	"github.com/altessa-s/go-atlas/cache/storages/lru"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// TestGetWithFallback_ReaderAfterDeleteDoesNotJoinOldFlight pins that a reader
// starting after Delete completes never receives the result of a load that read
// the source before the invalidation.
func TestGetWithFallback_ReaderAfterDeleteDoesNotJoinOldFlight(t *testing.T) {
	t.Parallel()

	const key = "user:1"

	storage, err := lru.New(16)
	require.NoError(t, err)
	c := cache.New(storage)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		v, loadErr := cache.GetWithFallbackT(ctx, c, key, func(context.Context) (string, time.Duration, error) {
			close(started)
			<-release
			return "old", cache.TTLUseDefault, nil
		})
		done <- result{v, loadErr}
	}()

	<-started
	require.NoError(t, c.Delete(ctx, key))

	// The second reader either runs its own fallback (signals ran) or joined
	// the old flight and is released only with it.
	ran := make(chan struct{})
	second := make(chan result, 1)
	go func() {
		v, loadErr := cache.GetWithFallbackT(ctx, c, key, func(context.Context) (string, time.Duration, error) {
			close(ran)
			return "new", cache.TTLUseDefault, nil
		})
		second <- result{v, loadErr}
	}()

	select {
	case <-ran:
	case res := <-second:
		close(release)
		require.NoError(t, res.err)
		require.Equal(t, "new", res.value, "a reader after Delete must not receive the old flight's value")
		require.Equal(t, "old", (<-done).value)
		return
	}
	res := <-second
	require.NoError(t, res.err)
	require.Equal(t, "new", res.value)

	close(release)
	old := <-done
	require.NoError(t, old.err)
	require.Equal(t, "old", old.value)
}

// TestGetWithFallback_FreshFlightAfterDeleteIsCached pins that a load that
// began after Delete stores its result although an older load of the key is
// still running.
func TestGetWithFallback_FreshFlightAfterDeleteIsCached(t *testing.T) {
	t.Parallel()

	storage, err := lru.New(16)
	require.NoError(t, err)
	c := cache.New(storage)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (string, time.Duration, error) {
			close(started)
			<-release
			return "old", cache.TTLUseDefault, nil
		})
	}()
	<-started
	require.NoError(t, c.Delete(ctx, "k"))

	got, err := cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (string, time.Duration, error) {
		return "new", cache.TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, "new", got)

	var cached string
	require.NoError(t, c.Get(ctx, "k", &cached))
	require.Equal(t, "new", cached)

	close(release)
	<-done
}

// TestSave_InvalidatesOverlappingLoadFlights pins that a Save separates later
// readers from a load that read the source before it.
func TestSave_InvalidatesOverlappingLoadFlights(t *testing.T) {
	t.Parallel()

	storage, err := lru.New(16)
	require.NoError(t, err)
	c := cache.New(storage)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (string, time.Duration, error) {
			close(started)
			<-release
			return "old", cache.TTLUseDefault, nil
		})
	}()
	<-started
	require.NoError(t, c.Save(ctx, "k", "saved"))
	require.NoError(t, c.Delete(ctx, "k"))

	ran := make(chan struct{})
	res := make(chan string, 1)
	go func() {
		v, _ := cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (string, time.Duration, error) {
			close(ran)
			return "new", cache.TTLUseDefault, nil
		})
		res <- v
	}()
	select {
	case <-ran:
	case v := <-res:
		close(release)
		require.Equal(t, "new", v)
		<-done
		return
	}
	require.Equal(t, "new", <-res)
	close(release)
	<-done
}

type user struct{ Name string }

// TestGetWithFallback_PointerDestination pins that a pointer-typed destination
// is filled on a miss, a hit and a fresh read.
func TestGetWithFallback_PointerDestination(t *testing.T) {
	t.Parallel()

	fb := func(context.Context) (any, time.Duration, error) {
		return &user{Name: "ann"}, cache.TTLUseDefault, nil
	}
	tests := []struct {
		name string
		ctx  func(context.Context) context.Context
		warm bool
		fb   cache.Fallback
	}{
		{name: "miss", ctx: func(c context.Context) context.Context { return c }, fb: fb},
		{name: "hit", ctx: func(c context.Context) context.Context { return c }, warm: true, fb: fb},
		{name: "fresh", ctx: cache.WithFreshReads, fb: fb},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage, err := lru.New(16)
			require.NoError(t, err)
			c := cache.New(storage)
			ctx := t.Context()
			if tc.warm {
				require.NoError(t, c.Save(ctx, "u", &user{Name: "ann"}))
			}

			var u *user
			require.NotPanics(t, func() {
				require.NoError(t, c.GetWithFallback(tc.ctx(ctx), "u", &u, tc.fb))
			})
			require.NotNil(t, u)
			require.Equal(t, "ann", u.Name)
		})
	}
}

// TestGetWithFallback_IncompatibleDestination pins that a destination the
// fallback result cannot be assigned to is an error, not a panic.
func TestGetWithFallback_IncompatibleDestination(t *testing.T) {
	t.Parallel()

	storage, err := lru.New(16)
	require.NoError(t, err)
	c := cache.New(storage)

	var n int
	require.NotPanics(t, func() {
		err = c.GetWithFallback(cache.WithFreshReads(t.Context()), "k", &n, func(context.Context) (any, time.Duration, error) {
			return "text", cache.TTLUseDefault, nil
		})
	})
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

// TestFailClosed_EntryRejectedReturnsLoadedValue pins that a deterministic
// storage write rejection does not turn a successful load into a failure.
func TestFailClosed_EntryRejectedReturnsLoadedValue(t *testing.T) {
	t.Parallel()

	c := cache.New(freecache.New(freecache.WithMaxSize(1 << 20)))
	big := strings.Repeat("x", 2048)

	for range 2 {
		got, err := cache.GetWithFallbackT(t.Context(), c, "big", func(context.Context) (string, time.Duration, error) {
			return big, cache.TTLUseDefault, nil
		})
		require.NoError(t, err)
		require.Equal(t, big, got)
	}
}

// TestFailClosed_TTLOutOfRangeReturnsLoadedValue pins that a fallback TTL the
// storage cannot represent does not fail the read.
func TestFailClosed_TTLOutOfRangeReturnsLoadedValue(t *testing.T) {
	t.Parallel()

	c := cache.New(freecache.New())
	got, err := cache.GetWithFallbackT(t.Context(), c, "k", func(context.Context) (string, time.Duration, error) {
		return "v", freecache.MaxTTL + time.Hour, nil
	})
	require.NoError(t, err)
	require.Equal(t, "v", got)
}
