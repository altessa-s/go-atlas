// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"slices"
	"sync"
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

	// Manually steer entries: one TransientFailure, one Ready. A non-nil
	// cancel marks the entry as watched, so its recorded state is used.
	tr.mu.Lock()
	for pc, entry := range tr.perTarget["api"] {
		entry.cancel = func() {}
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

	// Pre-mark an entry with a non-nil cancel so the implicit enable() in
	// subscribe sees a "watcher already running" sentinel and does not
	// spawn its own goroutine. This isolates fan-out from watcher races.
	entry := &trackedEntry{state: connectivity.Idle, cancel: func() {}}
	tr.mu.Lock()
	tr.perTarget["api"] = map[*pooledConnection]*trackedEntry{(*pooledConnection)(nil): entry}
	tr.mu.Unlock()

	var got1, got2 atomic.Int32
	unsub1 := tr.subscribe("api", func(s connectivity.State) { got1.Store(int32(s)) })
	unsub2 := tr.subscribe("api", func(s connectivity.State) { got2.Store(int32(s)) })

	tr.recordAndFanOut("api", entry, connectivity.Ready)
	testhelpers.WaitFor(t, time.Second, func() bool {
		return got1.Load() == int32(connectivity.Ready) && got2.Load() == int32(connectivity.Ready)
	}, "subscribers did not receive Ready")

	unsub1()
	tr.recordAndFanOut("api", entry, connectivity.TransientFailure)
	testhelpers.WaitFor(t, time.Second, func() bool {
		return got2.Load() == int32(connectivity.TransientFailure)
	}, "remaining subscriber did not receive TransientFailure")
	require.Equal(t, int32(connectivity.Ready), got1.Load(), "unsubscribed callback was invoked")
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
	p.tracker.perTarget["api"][pcReady].cancel = func() {}
	p.tracker.perTarget["backup"][pcDown].state = connectivity.Shutdown
	p.tracker.perTarget["backup"][pcDown].cancel = func() {}
	p.tracker.mu.Unlock()

	require.Equal(t, health.StatusNotServing, p.health.aggregateStatus())
}

func TestPoolHealthPerTargetRegistration(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := New(
		WithHealthCoordinator(coord),
		WithHealthPerTarget(),
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
		WithHealthPerTarget(),
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

func TestStoppedPoolReleasesOnlyItsRegistrations(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)
	reporter := struct{ health.DynamicReporter }{coord}
	newPool := func() (*ConnectionPool, func()) {
		p := New(WithHealthCoordinator(reporter), WithHealthPerTarget())
		stop, err := p.Start(t.Context())
		require.NoError(t, err)
		t.Cleanup(stop)
		p.health.onConnAttached("api")
		return p, stop
	}
	old, stopOld := newPool()
	_, stopCurrent := newPool()

	stopOld()
	require.Equal(t, health.StatusNotServing, old.health.CheckHealth(t.Context()))
	require.ElementsMatch(t, []string{DefaultPoolHealthServiceName, DefaultPoolHealthServiceName + ".api"}, slices.Collect(coord.ListServices()))

	stopCurrent()
	require.Empty(t, slices.Collect(coord.ListServices()))
}

type legacyPoolReporter struct {
	mu         sync.Mutex
	registered map[string]health.Checker
}

func (r *legacyPoolReporter) RegisterService(name string, checker health.Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered[name] = checker
}

func (r *legacyPoolReporter) NotifyStatusChange(string, health.ServingStatus) {}

func (r *legacyPoolReporter) UnregisterService(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.registered, name)
}

func (r *legacyPoolReporter) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.registered))
	for name := range r.registered {
		names = append(names, name)
	}
	return names
}

func TestPoolUnregistersFromLegacyDynamicReporter(t *testing.T) {
	t.Parallel()
	reporter := &legacyPoolReporter{registered: make(map[string]health.Checker)}
	p := New(WithHealthCoordinator(reporter), WithHealthPerTarget())
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	p.health.onConnAttached("api")
	require.ElementsMatch(t, []string{DefaultPoolHealthServiceName, DefaultPoolHealthServiceName + ".api"}, reporter.names())
	p.health.onConnDetached("api")
	require.Equal(t, []string{DefaultPoolHealthServiceName}, reporter.names())
	stop()
	require.Empty(t, reporter.names())
}

func TestExplicitScopedHealthReporter(t *testing.T) {
	t.Parallel()
	first, last := health.New(), health.New()
	t.Cleanup(first.Close)
	t.Cleanup(last.Close)
	reporter := struct{ health.ScopedReporter }{last}
	p := New(WithHealthCoordinator(first), WithScopedHealthReporter(reporter), WithHealthPerTarget())
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	p.health.onConnAttached("api")
	require.Empty(t, slices.Collect(first.ListServices()))
	require.ElementsMatch(t, []string{DefaultPoolHealthServiceName, DefaultPoolHealthServiceName + ".api"}, slices.Collect(last.ListServices()))
	stop()
	require.Empty(t, slices.Collect(last.ListServices()))
}

func TestSubscriberQueueIsBoundedAndKeepsLatest(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var mu sync.Mutex
	var got []connectivity.State
	sub := newSubscriber(func(s connectivity.State) {
		<-release
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
	})
	t.Cleanup(sub.close)

	states := []connectivity.State{connectivity.Ready, connectivity.TransientFailure}
	for i := range 10 * subscriberQueueSize {
		sub.enqueue(states[i%2])
	}
	sub.enqueue(connectivity.Shutdown)
	sub.mu.Lock()
	queued := len(sub.queue)
	sub.mu.Unlock()
	require.LessOrEqual(t, queued, subscriberQueueSize)

	close(release)
	testhelpers.WaitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) > 0 && got[len(got)-1] == connectivity.Shutdown
	}, "latest state was not delivered")
}

// TestSubscribeAfterShutdownRegistersNothing races registration with
// shutdown: whichever order wins, no subscriber may outlive shutdown.
func TestSubscribeAfterShutdownRegistersNothing(t *testing.T) {
	t.Parallel()

	for range 200 {
		tr := newStateTracker()
		var wg sync.WaitGroup
		wg.Go(func() { tr.subscribe("api", func(connectivity.State) {}) })
		wg.Go(tr.shutdown)
		wg.Wait()

		tr.subsMu.RLock()
		left := len(tr.subs)
		tr.subsMu.RUnlock()
		require.Zero(t, left, "a subscriber was registered after shutdown swept")
	}

	tr := newStateTracker()
	tr.shutdown()
	tr.subscribe("api", func(connectivity.State) {})
	tr.subsMu.RLock()
	defer tr.subsMu.RUnlock()
	require.Empty(t, tr.subs)
}

// TestDetachNotifiesTargetSubscribers removes the Ready connection of a target
// whose other connection is failing: subscribers must learn the aggregate
// changed.
func TestDetachNotifiesTargetSubscribers(t *testing.T) {
	t.Parallel()
	tr := newStateTracker()
	t.Cleanup(tr.shutdown)

	pcReady := newTrackedConn(t)
	pcDown := newTrackedConn(t)
	tr.attach("api", pcReady)
	tr.attach("api", pcDown)
	tr.mu.Lock()
	for pc, entry := range tr.perTarget["api"] {
		entry.cancel = func() {} // watched: the recorded state is used
		entry.state = connectivity.TransientFailure
		if pc == pcReady {
			entry.state = connectivity.Ready
		}
	}
	tr.mu.Unlock()

	var mu sync.Mutex
	var got []connectivity.State
	unsub := tr.subscribe("api", func(s connectivity.State) {
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
	})
	t.Cleanup(unsub)

	tr.detach("api", pcReady)

	testhelpers.WaitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) > 0 && got[len(got)-1] == connectivity.TransientFailure
	}, "subscriber was not told the target aggregate changed")
}
