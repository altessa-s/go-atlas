// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// startCapturing returns a Leader wired to a provider that records the
// transition channels Start hands it, so a test can publish leadership edges
// the way a real provider would.
func startCapturing(tb testing.TB, le *leadelect.Leader, prov *mockProvider) providers.Config {
	tb.Helper()

	captured := make(chan providers.Config, 1)
	prov.startFn = func(_ context.Context, cfg providers.Config) error {
		captured <- cfg
		return nil
	}

	require.NoError(tb, le.Start(tb.Context()))
	tb.Cleanup(func() { _ = le.Stop(context.Background()) })

	return <-captured
}

// trySend mirrors a provider publishing one leadership edge without blocking.
func trySend(tb testing.TB, ch chan<- struct{}) bool {
	tb.Helper()

	select {
	case ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// TestLeader_Stop_JoinsDispatchGoroutine pins that Stop terminates the dispatch
// goroutine itself rather than leaving it parked until the caller's context is
// canceled — the provider's StopCh notification is best-effort and must not be
// the only thing that ends it.
//
// Serial: it scans every goroutine in the process, so a parallel test running
// its own election would make the assertion meaningless.
func TestLeader_Stop_JoinsDispatchGoroutine(t *testing.T) {
	dispatchAlive := func() bool {
		buf := make([]byte, 1<<20)
		return strings.Contains(string(buf[:runtime.Stack(buf, true)]), "leadelect.(*Leader).dispatch")
	}

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	prov, err := lenats.New(t.Context(), nc, lenats.WithBucket("lifecycle-join"))
	require.NoError(t, err)

	le := leadelect.New(prov, "lifecycle-join", "node-1", leadelect.WithTTL(2*time.Second))
	require.NoError(t, le.Start(t.Context()))

	require.Eventually(t, le.IsLeader, 3*time.Second, 50*time.Millisecond, "should acquire leadership")
	require.True(t, dispatchAlive(), "dispatch goroutine should be running while the election is active")

	require.NoError(t, le.Stop(t.Context()))
	require.False(t, dispatchAlive(), "dispatch goroutine must not outlive Stop")
}

// TestLeader_Stop_DeliversStopNotification pins that the provider's terminal
// notification reaches an unbuffered consumer. A select-with-default send lost
// it whenever the consumer was not already parked in its receive.
func TestLeader_Stop_DeliversStopNotification(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	prov, err := lenats.New(t.Context(), nc, lenats.WithBucket("lifecycle-stopch"))
	require.NoError(t, err)

	stopCh := make(chan struct{})
	received := make(chan struct{}, 1)
	go func() {
		<-stopCh
		received <- struct{}{}
	}()

	require.NoError(t, prov.Start(t.Context(), providers.Config{
		Key:      "lifecycle-stopch",
		TTL:      2 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   stopCh,
	}))
	require.Eventually(t, prov.IsLeader, 3*time.Second, 50*time.Millisecond, "should acquire leadership")
	require.NoError(t, prov.Stop(t.Context()))

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("StopCh was never signaled after Stop")
	}
}

// TestLeader_LostTransition_SurvivesBusyDispatcher pins that a leadership-lost
// edge raised while a previous callback is still running is queued rather than
// dropped. Losing that edge leaves the application acting as leader after it
// no longer is.
func TestLeader_LostTransition_SurvivesBusyDispatcher(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{}
	le := leadelect.New(prov, "test", "n1", leadelect.WithHandlerTimeout(5*time.Second))

	inCallback := make(chan struct{})
	le.RegisterOnBecomesLeader(func(_ context.Context, _ leadelect.LeaderElector) {
		close(inCallback)
		time.Sleep(500 * time.Millisecond)
	})

	var lostCalls atomic.Int32
	le.RegisterOnLeaderLost(func(_ context.Context, _ leadelect.LeaderElector) { lostCalls.Add(1) })

	cfg := startCapturing(t, le, prov)

	require.True(t, trySend(t, cfg.BecameCh), "became-leader edge should be accepted")
	<-inCallback // the dispatcher is now occupied inside the callback

	require.True(t, trySend(t, cfg.LostCh), "leadership-lost edge must not be dropped while the dispatcher is busy")

	require.Eventually(t, func() bool { return lostCalls.Load() == 1 },
		3*time.Second, 20*time.Millisecond, "onLeaderLost should run once the dispatcher frees up")
}

// TestLeader_Stop_QuiescesCallbacks pins Stop's documented contract: once it
// returns, no callback is executing and none will start.
//
// Stop is not required to *run* a transition it raced — skipping a not-yet-
// started callback is the correct shutdown behavior. What it must never do is
// return while one is mid-flight. Registering the wait group only inside the
// callback path left exactly that window, between the channel receive and the
// registration, so Stop observed a zero counter and returned early while the
// callback carried on. The assertion is therefore stability: whatever the race
// outcome, the observed state must not change after Stop has returned.
func TestLeader_Stop_QuiescesCallbacks(t *testing.T) {
	t.Parallel()

	const attempts = 100

	for i := range attempts {
		prov := &mockProvider{}
		le := leadelect.New(prov, "test", "n1")

		var ran atomic.Bool
		le.RegisterOnBecomesLeader(func(_ context.Context, _ leadelect.LeaderElector) {
			time.Sleep(10 * time.Millisecond)
			ran.Store(true)
		})

		cfg := startCapturing(t, le, prov)
		require.True(t, trySend(t, cfg.BecameCh))

		require.NoError(t, le.Stop(t.Context()))

		settled := ran.Load()
		time.Sleep(30 * time.Millisecond)
		require.Equal(t, settled, ran.Load(), "a callback was still running after Stop returned (attempt %d)", i)
	}
}

// TestLeader_Stop_ClearsLeaderGauge pins that a stopped elector stops reporting
// itself as leader; a stuck gauge shows two leaders on a dashboard.
func TestLeader_Stop_ClearsLeaderGauge(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	prov := &mockProvider{}
	le := leadelect.New(prov, "test", "n1", leadelect.WithCollector(tc))

	cfg := startCapturing(t, le, prov)
	require.True(t, trySend(t, cfg.BecameCh))

	const gauge = "test_leader_election_is_leader"
	require.Eventually(t, func() bool { return testhelpers.GetGaugeValue(t, tc, gauge) == 1 },
		2*time.Second, 20*time.Millisecond, "gauge should report leadership")

	require.NoError(t, le.Stop(t.Context()))
	require.Equal(t, float64(0), testhelpers.GetGaugeValue(t, tc, gauge), "gauge must be cleared on Stop")
}
