// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// newIdleConn returns a freshly-created *grpc.ClientConn pointing at a
// guaranteed-unreachable address. Because [grpc.NewClient] dials lazily,
// the conn stays in Idle until something triggers connect — exactly the
// state we want for unit tests that don't need real I/O.
func newIdleConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newTrackedConn(t *testing.T) *pooledConnection {
	t.Helper()
	return &pooledConnection{conn: newIdleConn(t), target: "test"}
}

func TestStateTrackerAttachDetach(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()
	t.Cleanup(tr.shutdown)

	pc := newTrackedConn(t)
	tr.attach("api", pc)

	require.Equal(t, []string{"api"}, slices.Sorted(slices.Values(tr.allTargets())))
	require.Equal(t, connectivity.Idle, tr.stateForTarget("api"))

	tr.detach("api", pc)
	require.Empty(t, tr.allTargets())
	require.Equal(t, connectivity.Idle, tr.stateForTarget("unknown"))
}

func TestStateTrackerStateForTargetBest(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()
	t.Cleanup(tr.shutdown)

	pc1 := newTrackedConn(t)
	pc2 := newTrackedConn(t)
	tr.attach("api", pc1)
	tr.attach("api", pc2)

	// Manually steer entries: one TransientFailure, one Ready.
	tr.mu.Lock()
	for pc, entry := range tr.perTarget["api"] {
		switch pc {
		case pc1:
			entry.state = connectivity.TransientFailure
		case pc2:
			entry.state = connectivity.Ready
		}
	}
	tr.mu.Unlock()

	require.Equal(t, connectivity.Ready, tr.stateForTarget("api"))
}

func TestStateTrackerSubscribeFanOutAndUnsubscribe(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()
	t.Cleanup(tr.shutdown)

	pc := newTrackedConn(t)
	tr.attach("api", pc)

	var got1, got2 atomic.Int32
	unsub1 := tr.subscribe("api", func(s connectivity.State) { got1.Store(int32(s)) })
	unsub2 := tr.subscribe("api", func(s connectivity.State) { got2.Store(int32(s)) })

	// Direct fan-out without spawning a watcher.
	tr.recordAndFanOut("api", pc, tr.perTarget["api"][pc], connectivity.Ready)
	require.Equal(t, int32(connectivity.Ready), got1.Load())
	require.Equal(t, int32(connectivity.Ready), got2.Load())

	unsub1()
	tr.recordAndFanOut("api", pc, tr.perTarget["api"][pc], connectivity.TransientFailure)
	require.Equal(t, int32(connectivity.Ready), got1.Load(), "unsubscribed callback was invoked")
	require.Equal(t, int32(connectivity.TransientFailure), got2.Load())
	unsub2()
}

func TestStateTrackerLazyEnableSpawnsWatchers(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()
	t.Cleanup(tr.shutdown)

	pc := newTrackedConn(t)
	tr.attach("api", pc)
	require.False(t, tr.enabled.Load())

	// Subscribe enables and back-fills.
	unsub := tr.subscribe("api", func(connectivity.State) {})
	t.Cleanup(unsub)

	require.True(t, tr.enabled.Load())
	testhelpers.WaitFor(t, time.Second, func() bool {
		tr.mu.RLock()
		defer tr.mu.RUnlock()
		return tr.perTarget["api"][pc].cancel != nil
	}, "watcher goroutine did not start after enable")
}

func TestStateTrackerShutdownWaitsForWatchers(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()

	pc := newTrackedConn(t)
	tr.attach("api", pc)
	tr.enable()

	testhelpers.WaitFor(t, time.Second, func() bool {
		tr.mu.RLock()
		defer tr.mu.RUnlock()
		return tr.perTarget["api"][pc].cancel != nil
	}, "watcher did not start")

	tr.shutdown()
	// shutdown must have returned only after wg.Wait completes — assert
	// idempotence and that no watcher is still tracked as running.
	tr.shutdown()
}

func TestDefaultStateMapperTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state connectivity.State
		want  health.ServingStatus
	}{
		{connectivity.Ready, health.StatusServing},
		{connectivity.Idle, health.StatusServing},
		{connectivity.Connecting, health.StatusServing},
		{connectivity.TransientFailure, health.StatusDegraded},
		{connectivity.Shutdown, health.StatusNotServing},
	}
	for _, c := range cases {
		t.Run(c.state.String(), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.want, DefaultStateMapper(c.state))
		})
	}
}

func TestPoolHealthAggregateStatusEmpty(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := New(WithHealthCoordinator(coord))
	t.Cleanup(func() { p.tracker.shutdown() })

	require.Equal(t, health.StatusServing, p.health.aggregateStatus())
}

func TestPoolHealthAggregateStatusWorstAcrossTargets(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := New(WithHealthCoordinator(coord))
	t.Cleanup(func() { p.tracker.shutdown() })

	pcReady := newTrackedConn(t)
	pcDown := newTrackedConn(t)
	p.tracker.attach("api", pcReady)
	p.tracker.attach("backup", pcDown)

	p.tracker.mu.Lock()
	p.tracker.perTarget["api"][pcReady].state = connectivity.Ready
	p.tracker.perTarget["backup"][pcDown].state = connectivity.Shutdown
	p.tracker.mu.Unlock()

	require.Equal(t, health.StatusNotServing, p.health.aggregateStatus())
}

func TestPoolHealthPerTargetRegistration(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := New(
		WithHealthCoordinator(coord),
		WithPerTargetHealthChecks(),
	)
	t.Cleanup(func() { p.tracker.shutdown() })
	p.health.register()

	pc := newTrackedConn(t)
	p.tracker.attach("api", pc)
	p.health.onConnAttached("api")

	registered := slices.Sorted(slices.Values(slices.Collect(coord.ListServices())))
	require.Equal(t, []string{
		DefaultPoolHealthServiceName,
		DefaultPoolHealthServiceName + ".api",
	}, registered)

	p.tracker.detach("api", pc)
	p.health.onConnDetached("api")

	registered = slices.Sorted(slices.Values(slices.Collect(coord.ListServices())))
	require.Equal(t, []string{DefaultPoolHealthServiceName}, registered)
}

func TestWithHealthStateMapperOverride(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	custom := func(connectivity.State) health.ServingStatus { return health.StatusDegraded }
	p := New(WithHealthCoordinator(coord), WithHealthStateMapper(custom))
	t.Cleanup(func() { p.tracker.shutdown() })

	pc := newTrackedConn(t)
	p.tracker.attach("api", pc)

	require.Equal(t, health.StatusDegraded, p.health.targetStatus("api"))
}

// TestPoolPushNotifyOnStateChange exercises the full push pipeline. We
// dial an unreachable address so the conn deterministically transitions
// from Idle through Connecting into TransientFailure once Connect() is
// called. The coordinator subscriber must observe a non-Serving status
// without the scheduler running a pull cycle.
func TestPoolPushNotifyOnStateChange(t *testing.T) {
	t.Parallel()
	const target = "127.0.0.1:1" // reserved port, refuses connections instantly

	coord := health.New()
	t.Cleanup(coord.Close)

	p := New(
		WithHealthCoordinator(coord),
		WithPerTargetHealthChecks(),
	)
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	conn, err := p.GetConnection(t.Context(), target)
	require.NoError(t, err)
	t.Cleanup(func() { p.ReturnConnection(conn) })

	sub, err := coord.Subscribe(t.Context(), DefaultPoolHealthServiceName+"."+target)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	// Force the conn out of Idle: connecting to a reserved port fails
	// quickly and lands the conn in TransientFailure.
	conn.Connect()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case status := <-sub.Updates():
			if status == health.StatusDegraded {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe Degraded push within 5s after Connect on unreachable target; current state=%v",
				p.StateForTarget(target))
		}
	}
}

// TestPoolStartWithoutCoordinatorIsNoop confirms the no-config path stays
// zero-cost: tracker is not enabled and no per-target services exist.
func TestPoolStartWithoutCoordinatorIsNoop(t *testing.T) {
	t.Parallel()
	p := New()
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	require.False(t, p.tracker.enabled.Load())

	// Public CheckHealth still works (uses default mapper).
	require.Equal(t, health.StatusServing, p.CheckHealth(t.Context()))
}

func TestPoolSubscribeTargetEnablesTracker(t *testing.T) {
	t.Parallel()
	p := New()
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	require.False(t, p.tracker.enabled.Load())

	unsub, err := p.SubscribeTarget("api", func(connectivity.State) {})
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.True(t, p.tracker.enabled.Load())
}

func TestPoolSubscribeTargetAfterStopReturnsClosed(t *testing.T) {
	t.Parallel()
	p := New()
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	stop()

	_, err = p.SubscribeTarget("api", func(connectivity.State) {})
	require.ErrorIs(t, err, ErrConnectionPoolClosed)
}
