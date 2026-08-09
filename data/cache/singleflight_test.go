// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/providers/lru"
)

func sfCache(tb testing.TB) *cache.Cache {
	tb.Helper()

	provider, err := lru.New(128)
	require.NoError(tb, err)

	return cache.New(provider)
}

// TestGetWithFallback_LosingCallerLeavesOnItsOwnContext pins that a caller
// whose request is cancelled stops waiting on the shared fetch instead of
// staying pinned to it until it finishes.
func TestGetWithFallback_LosingCallerLeavesOnItsOwnContext(t *testing.T) {
	t.Parallel()

	c := sfCache(t)

	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once

	fallback := func() (any, time.Duration, error) {
		once.Do(func() { close(entered) })
		<-release

		return "value", cache.TTLUseDefault, nil
	}

	// Winner: holds the group open until release.
	var winner sync.WaitGroup
	winner.Go(func() {
		var out string
		_ = c.GetWithFallback(t.Context(), "key", &out, fallback)
	})
	<-entered

	// Latecomer: collapses onto the in-flight group, then gives up.
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	var out string
	err := c.GetWithFallback(ctx, "key", &out, fallback)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded, "the waiter should fail on its own deadline")
	require.Less(t, elapsed, 2*time.Second, "the waiter stayed pinned to the shared fetch")

	close(release)
	winner.Wait()
}

// TestGetWithFallback_WinnerCancellationDoesNotPoisonWaiters pins the other
// half of the singleflight trap: the caller that happens to win the group owns
// the shared fetch, so its cancellation must not fail everyone queued behind
// it. The shared write runs on a context detached from the winner's.
func TestGetWithFallback_WinnerCancellationDoesNotPoisonWaiters(t *testing.T) {
	t.Parallel()

	c := sfCache(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	fallback := func() (any, time.Duration, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}

		return "value", cache.TTLUseDefault, nil
	}

	winnerCtx, cancelWinner := context.WithCancel(t.Context())

	var winner sync.WaitGroup
	winner.Go(func() {
		var out string
		_ = c.GetWithFallback(winnerCtx, "key", &out, fallback)
	})
	<-entered

	// A waiter joins the in-flight group with a healthy context of its own.
	waiterDone := make(chan error, 1)
	var waited string
	go func() {
		waiterDone <- c.GetWithFallback(t.Context(), "key", &waited, fallback)
	}()

	// Give the waiter a moment to collapse onto the group, then kill the
	// winner's request while the shared fetch is still running.
	time.Sleep(100 * time.Millisecond)
	cancelWinner()
	close(release)

	select {
	case err := <-waiterDone:
		require.NoError(t, err, "the winner's cancellation must not fail a healthy waiter")
		require.Equal(t, "value", waited)
	case <-time.After(5 * time.Second):
		t.Fatal("waiter never completed")
	}

	winner.Wait()
}
