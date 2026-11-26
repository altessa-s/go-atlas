// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	src.gated.Store(true)
	require.NoError(t, m.StartWatching(t.Context()))
	second := <-src.ctxs
	require.NotEqual(t, first, second, "the restarted loop must run under its own context")
	require.NoError(t, second.Err(), "the second loop's context must be live while watching")
	stopped := make(chan struct{})
	go func() {
		m.StopWatching()
		close(stopped)
	}()
	<-src.canceled
	src.gated.Store(false)
	close(src.release)
	<-stopped
	require.ErrorIs(t, second.Err(), context.Canceled)

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

func TestCloseJoinsScheduledAndManualReload(t *testing.T) {
	t.Parallel()
	for _, scheduled := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			src := newGatedSource()
			m := newGatedManager(t, src)
			run := m.RunUpdateCycle
			if scheduled {
				run = m.RegisterSchedulerFunc()
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			src.gated.Store(true)
			finished := make(chan error, 1)
			go func() { finished <- run(ctx) }()
			<-src.entered
			closed := make(chan error, 1)
			go func() { closed <- m.Close() }()
			synctest.Wait()
			canceled := len(src.canceled)
			count := src.closeCalls.Load()
			cancel()
			close(src.release)
			require.NoError(t, <-finished)
			require.NoError(t, <-closed)
			require.Equal(t, 1, canceled)
			require.Zero(t, count)
			require.Zero(t, src.violations.Load())
		})
	}
}

func TestWatchContextExitAndRestart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := newGatedSource()
		m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithPollInterval(time.Hour))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		require.NoError(t, m.StartWatching(ctx))
		cancel()
		synctest.Wait()
		require.False(t, m.IsWatching())
		require.NoError(t, m.StartWatching(t.Context()))
		require.True(t, m.IsWatching())
		require.NoError(t, m.Close())
		require.False(t, m.IsWatching())
	})
}

func TestScheduledManagersIndependent(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	opts := []opa.Option{opa.WithScheduler(reg), opa.WithUpdateSchedule("@every 1m", false)}
	first, second := newGatedSource(), newGatedSource()
	m1, err := opa.NewManager(t.Context(), first, "data.t.allow", opts...)
	require.NoError(t, err)
	m2, err := opa.NewManager(t.Context(), second, "data.t", opts...)
	require.NoError(t, err)
	defer m2.Close()
	require.NoError(t, m1.Close())
	require.NotEqual(t, reg.tasks[0].ID, reg.tasks[1].ID)
	require.False(t, m1.SchedulerCallbackAttached())
	require.True(t, m2.SchedulerCallbackAttached())
	require.NoError(t, reg.tasks[0].Func(t.Context()))
	require.NoError(t, reg.tasks[1].Func(t.Context()))
	require.EqualValues(t, 1, first.fetches.Load())
	require.EqualValues(t, 2, second.fetches.Load())
}

func TestCloseChainedWatchers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := newGatedSource()
		m := newGatedManager(t, src)
		sub, err := m.Watch(t.Context(), opa.WatchOptions{})
		require.NoError(t, err)
		src.gated.Store(true)
		require.NoError(t, m.StartWatching(t.Context()))
		<-src.entered
		stopped := make(chan struct{})
		go func() {
			m.StopWatching()
			close(stopped)
		}()
		<-src.canceled
		require.NoError(t, m.StartWatching(t.Context()))
		closed := make(chan error, 1)
		go func() { closed <- m.Close() }()
		synctest.Wait()
		signaled := false
		select {
		case <-sub.Done:
			signaled = true
		default:
		}
		close(src.release)
		<-stopped
		require.NoError(t, <-closed)
		require.True(t, signaled, "subscriptions must stop admitting deliveries before joining chained watchers")
		require.False(t, m.IsWatching())
		require.Zero(t, src.violations.Load())
	})
}

func TestWatchBlockedDeliveryDuringClose(t *testing.T) {
	t.Parallel()
	for _, closeManager := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			src := &mutableSource{bundle: adminsBundle("a")}
			m, err := opa.NewManager(t.Context(), src, "data.t.allow")
			require.NoError(t, err)
			sub, err := m.Watch(t.Context(), opa.WatchOptions{BufferSize: 1, BufferOverflowPolicy: opa.OverflowPolicyBlock})
			require.NoError(t, err)
			update(t, m, src, "b") // Fill the subscriber so the next send must wait.
			src.set(adminsBundle("c"), nil)
			finished := make(chan error, 1)
			go func() { finished <- m.RunUpdateCycle(t.Context()) }()
			synctest.Wait()
			require.Empty(t, finished)
			if closeManager {
				require.NoError(t, m.Close())
			} else {
				sub.Stop()
			}
			require.NoError(t, <-finished)
			<-sub.Done
			require.Len(t, sub.Events, 1)
			<-sub.Events
			_, ok := <-sub.Events
			require.False(t, ok)
			require.NoError(t, m.Close())
		})
	}
}

func TestWatchPreviousExitPreservesRestart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := newGatedSource()
		m := newGatedManager(t, src)
		src.gated.Store(true)
		require.NoError(t, m.StartWatching(t.Context()))
		<-src.entered
		stopped := make(chan struct{})
		go func() { m.StopWatching(); close(stopped) }()
		<-src.canceled
		next, cancel := context.WithCancel(t.Context())
		defer cancel()
		require.NoError(t, m.StartWatching(next))
		src.gated.Store(false)
		close(src.release)
		<-stopped
		synctest.Wait()
		active := m.IsWatching()
		cancel()
		require.NoError(t, m.Close())
		require.True(t, active, "exiting poller cleared the replacement's state")
	})
}

func TestBlockingWatchContextCancellation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := &mutableSource{bundle: adminsBundle("a")}
		m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithPollInterval(time.Second))
		require.NoError(t, err)
		defer m.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		sub, err := m.Watch(ctx, opa.WatchOptions{BufferSize: 1, BufferOverflowPolicy: opa.OverflowPolicyBlock})
		require.NoError(t, err)
		update(t, m, src, "b")
		previous := m.Revision()
		src.set(adminsBundle("c"), nil)
		require.NoError(t, m.StartWatching(ctx))
		time.Sleep(time.Second)
		synctest.Wait()
		require.NotEqual(t, previous, m.Revision())
		cancel()
		stopped := make(chan struct{})
		go func() { m.StopWatching(); close(stopped) }()
		synctest.Wait()
		done := func(ch <-chan struct{}) bool {
			select {
			case <-ch:
				return true
			default:
				return false
			}
		}
		subDone, pollStopped := done(sub.Done), done(stopped)
		sub.Stop()
		<-stopped
		require.True(t, subDone, "watch subscription remained open after context cancellation")
		require.True(t, pollStopped, "StopWatching could not join the blocked poller")
	})
}

func TestWatchRejectsNegativeBufferSize(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("a")}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	require.NotPanics(t, func() {
		_, err = m.Watch(t.Context(), opa.WatchOptions{BufferSize: -1})
	})
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

func TestStopWatchingReleasesBlockedDeliveryToLiveSubscriber(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := &mutableSource{bundle: adminsBundle("a")}
		m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithPollInterval(time.Second))
		require.NoError(t, err)
		defer m.Close()
		sub, err := m.Watch(t.Context(), opa.WatchOptions{BufferSize: 1, BufferOverflowPolicy: opa.OverflowPolicyBlock})
		require.NoError(t, err)
		defer sub.Stop()
		update(t, m, src, "b")
		previous := m.Revision()
		src.set(adminsBundle("c"), nil)
		require.NoError(t, m.StartWatching(t.Context()))
		time.Sleep(time.Second)
		synctest.Wait()
		require.NotEqual(t, previous, m.Revision())

		stopped := make(chan struct{})
		go func() { m.StopWatching(); close(stopped) }()
		synctest.Wait()
		select {
		case <-stopped:
		default:
			sub.Stop()
			<-stopped
			require.FailNow(t, "StopWatching remained blocked on a live subscriber")
		}
		select {
		case <-sub.Done:
			require.FailNow(t, "stopping polling closed a live subscription")
		default:
		}
	})
}
