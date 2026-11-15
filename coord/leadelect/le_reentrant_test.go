// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// receiveErr returns the error a callback reported, failing the test when the
// lifecycle call it made never returned.
func receiveErr(t *testing.T, ch <-chan error, call string) error {
	t.Helper()

	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		require.FailNow(t, "lifecycle call from a callback deadlocked", call)
		return nil
	}
}

// A leader callback that gives up — a failed leader initialization, say — may
// stop the elector it runs on: Stop returns instead of waiting for itself, the
// election ends, the loss callbacks run once, and the elector can be started
// again.
func TestLeader_StopFromCallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		stop func(ctx context.Context, le *leadelect.Leader) error
	}{
		{name: "with the callback context", stop: func(ctx context.Context, le *leadelect.Leader) error { return le.Stop(ctx) }},
		{name: "with a context unrelated to the callback", stop: func(_ context.Context, le *leadelect.Leader) error {
			return le.Stop(t.Context())
		}},
		{name: "from a goroutine the callback waits for", stop: func(ctx context.Context, le *leadelect.Leader) error {
			done := make(chan error, 1)
			go func() { done <- le.Stop(ctx) }()
			return <-done
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prov := &mockProvider{nodeID: "node-1"}
			le := leadelect.New(prov, "election", "node-1")

			var lost atomic.Int32
			stopped := make(chan error, 1)
			le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) { stopped <- tc.stop(ctx, le) })
			le.RegisterOnLeaderLost(func(context.Context, leadelect.LeaderElector) { lost.Add(1) })

			cfg := startCapturing(t, le, prov)
			require.True(t, trySend(t, cfg.BecameCh))

			require.NoError(t, receiveErr(t, stopped, "Stop"))
			require.False(t, le.IsRunning())

			// A Stop from outside joins the dispatch goroutine the callback
			// left running.
			require.NoError(t, le.Stop(t.Context()))
			require.EqualValues(t, 1, lost.Load(), "the term the callback ended must be closed by one loss")

			require.NoError(t, le.Start(t.Context()))
			require.True(t, le.IsRunning())
			require.NoError(t, le.Stop(t.Context()))
			require.EqualValues(t, 1, lost.Load())
		})
	}
}

func TestLeader_StartFromCallbackIsRefused(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{nodeID: "node-1"}
	le := leadelect.New(prov, "election", "node-1")

	started := make(chan error, 1)
	le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) { started <- le.Start(ctx) })

	cfg := startCapturing(t, le, prov)
	require.True(t, trySend(t, cfg.BecameCh))

	require.ErrorIs(t, receiveErr(t, started, "Start"), leadelect.ErrStartFromCallback)
	require.True(t, le.IsRunning(), "a refused Start must leave the election running")
	require.NoError(t, le.Stop(t.Context()))
}

// A callback that calls Stop while a Stop from outside is already waiting for
// the callbacks returns at once instead of deadlocking with it.
func TestLeader_StopFromCallbackDuringStop(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{nodeID: "node-1"}
	le := leadelect.New(prov, "election", "node-1")

	entered := make(chan struct{})
	stopping := make(chan struct{})
	nested := make(chan error, 1)
	le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) {
		close(entered)
		<-stopping
		nested <- le.Stop(ctx)
	})
	// The provider's Stop runs under the outer Stop: by then that Stop holds
	// the lifecycle and goes on to wait for the callback.
	prov.stopFn = func(context.Context) error {
		close(stopping)
		return nil
	}

	cfg := startCapturing(t, le, prov)
	require.True(t, trySend(t, cfg.BecameCh))
	<-entered

	outer := make(chan error, 1)
	go func() { outer <- le.Stop(context.WithoutCancel(t.Context())) }()

	require.NoError(t, receiveErr(t, nested, "nested Stop"))
	require.NoError(t, receiveErr(t, outer, "outer Stop"))
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return !le.IsRunning() }, "the election did not stop")
}
