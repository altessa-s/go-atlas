// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
)

// gatedSource serves a fixed bundle. While gated, a Fetch signals entered,
// waits for its context to be canceled (only StopWatching/Close cancel a poll
// loop's context), signals canceled, and then keeps running until release is
// closed, like a slow source that does not abort promptly. It records Close
// calls and any Fetch that overlaps or follows Close.
type gatedSource struct {
	gated    atomic.Bool
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
	ctxs     chan context.Context // context of each Fetch (dropped when full)

	fetches    atomic.Int32
	inFetch    atomic.Int32
	closeCalls atomic.Int32
	violations atomic.Int32 // Fetch after Close, or Close during Fetch
}

func newGatedSource() *gatedSource {
	return &gatedSource{
		entered:  make(chan struct{}, 64),
		canceled: make(chan struct{}, 64),
		release:  make(chan struct{}),
		ctxs:     make(chan context.Context, 1024),
	}
}

func (s *gatedSource) Name() string { return "gated" }

func (s *gatedSource) Fetch(ctx context.Context) (*opa.PolicyBundle, error) {
	s.fetches.Add(1)
	select {
	case s.ctxs <- ctx:
	default:
	}
	s.inFetch.Add(1)
	defer s.inFetch.Add(-1)
	if s.closeCalls.Load() > 0 {
		s.violations.Add(1)
	}
	if s.gated.Load() {
		s.entered <- struct{}{}
		<-ctx.Done()
		s.canceled <- struct{}{}
		<-s.release
	}
	return &opa.PolicyBundle{
		Revision: "r1",
		Modules:  map[string][]byte{"p.rego": []byte("package t\nimport rego.v1\nallow if true\n")},
	}, nil
}

func (s *gatedSource) Close() error {
	if s.inFetch.Load() > 0 {
		s.violations.Add(1)
	}
	s.closeCalls.Add(1)
	return nil
}

func newGatedManager(t *testing.T, src *gatedSource) *opa.Manager {
	t.Helper()
	m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithPollInterval(time.Millisecond))
	require.NoError(t, err)
	return m
}

// requireStillWaiting asserts that waiter has not returned although the loop
// it must join is still inside Fetch. The caller has already observed the
// loop's cancellation, so the waiter is past its cancel step: a correct waiter
// never returns here (no false failures), while one that does not join returns
// within the grace period.
func requireStillWaiting(t *testing.T, waiter <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-waiter:
		require.Fail(t, msg)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestManager_StopWatchingJoinsPollLoop: StopWatching must not return while
// the poll loop is still inside Fetch.
func TestManager_StopWatchingJoinsPollLoop(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)
	t.Cleanup(func() { _ = m.Close() })

	src.gated.Store(true)
	require.NoError(t, m.StartWatching(t.Context()))
	<-src.entered // the loop is inside Fetch

	stopped := make(chan struct{})
	go func() {
		m.StopWatching()
		close(stopped)
	}()
	<-src.canceled // StopWatching canceled the loop; Fetch still runs
	requireStillWaiting(t, stopped, "StopWatching returned while the poll loop was in Fetch")

	src.gated.Store(false)
	close(src.release)
	<-stopped
	require.Zero(t, src.inFetch.Load())
}

// TestManager_ConcurrentCloseWaitsForTeardown: no Close caller returns before
// the teardown finished, and the source is closed exactly once, never while
// Fetch is running.
func TestManager_ConcurrentCloseWaitsForTeardown(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)

	src.gated.Store(true)
	require.NoError(t, m.StartWatching(t.Context()))
	<-src.entered

	first, second := make(chan struct{}), make(chan struct{})
	go func() { _ = m.Close(); close(first) }()
	<-src.canceled // the first Close has begun its teardown
	go func() { _ = m.Close(); close(second) }()
	requireStillWaiting(t, first, "Close returned while the poll loop was in Fetch")
	requireStillWaiting(t, second, "a concurrent Close returned before the teardown finished")
	require.Zero(t, src.closeCalls.Load(), "source closed while Fetch was running")

	src.gated.Store(false)
	close(src.release)
	<-first
	<-second
	require.Equal(t, int32(1), src.closeCalls.Load())
	require.Zero(t, src.violations.Load())
}

// TestManager_RestartUsesFreshContext observes each loop's Fetch contexts:
// the first loop's context is canceled once StopWatching returns, the second
// loop fetches under a different, live context, and once it is stopped no
// Fetch happens at all.
func TestManager_RestartUsesFreshContext(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)
	t.Cleanup(func() { _ = m.Close() })
	for len(src.ctxs) > 0 { // drop NewManager's initial load
		<-src.ctxs
	}

	require.NoError(t, m.StartWatching(t.Context()))
	first := <-src.ctxs
	m.StopWatching()
	require.Error(t, first.Err(), "the first loop's context must be canceled by StopWatching")

	// StopWatching joined the first loop, so every context recorded from now
	// on comes from the second loop.
	for len(src.ctxs) > 0 {
		<-src.ctxs
	}
	require.NoError(t, m.StartWatching(t.Context()))
	second := <-src.ctxs
	require.NotEqual(t, first, second, "the restarted loop must run under its own context")
	require.NoError(t, second.Err(), "the second loop's context must be live while watching")
	m.StopWatching()
	require.Error(t, second.Err())

	for len(src.ctxs) > 0 {
		<-src.ctxs
	}
	before := src.fetches.Load()
	time.Sleep(20 * time.Millisecond) // 20 poll intervals
	require.Equal(t, before, src.fetches.Load(), "a poll loop kept fetching after StopWatching")
	require.Zero(t, src.inFetch.Load())
}

// TestManager_RestartStormWithClose: stop/start storms racing Close leave no
// loop running once Close returns, never fetch after the source is closed, and
// refuse to start afterwards.
func TestManager_RestartStormWithClose(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 25 {
				_ = m.StartWatching(t.Context())
				m.StopWatching()
			}
		})
	}
	closed := make(chan struct{})
	wg.Go(func() {
		if err := m.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if src.inFetch.Load() != 0 || src.closeCalls.Load() != 1 {
			t.Error("Close returned before every poll loop exited and the source was closed")
		}
		close(closed)
	})
	wg.Wait()
	<-closed

	require.ErrorIs(t, m.StartWatching(t.Context()), opa.ErrManagerClosed)
	require.False(t, m.IsWatching())
	require.Zero(t, src.inFetch.Load())
	require.Zero(t, src.violations.Load(), "Fetch ran after (or during) source Close")
}
