// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// A leader whose Start context is canceled runs its loss callbacks exactly
// once, whatever the caller does next, and can be started again.
func TestLeader_StartContextCancelDeliversLossOnceAndRestarts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// afterCancel runs between the cancellation and the restart.
		afterCancel func(t *testing.T, le *leadelect.Leader, lost *atomic.Int32)
	}{
		{name: "restart at once", afterCancel: func(*testing.T, *leadelect.Leader, *atomic.Int32) {}},
		{name: "restart after the loss", afterCancel: func(t *testing.T, _ *leadelect.Leader, lost *atomic.Int32) {
			testhelpers.WaitFor(t, 10*time.Second, func() bool { return lost.Load() == 1 }, "the loss callback did not run after the context was canceled")
		}},
		{name: "stop then restart", afterCancel: func(t *testing.T, le *leadelect.Leader, _ *atomic.Int32) {
			require.NoError(t, le.Stop(t.Context()))
		}},
		{name: "loss then stop then restart", afterCancel: func(t *testing.T, le *leadelect.Leader, lost *atomic.Int32) {
			testhelpers.WaitFor(t, 10*time.Second, func() bool { return lost.Load() == 1 }, "the loss callback did not run after the context was canceled")
			require.NoError(t, le.Stop(t.Context()))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			le, err := lenats.NewLeader(t.Context(), natstest.ConnectNATS(t, ns), "svc", "node-1")
			require.NoError(t, err)

			var became, lost atomic.Int32
			le.RegisterOnBecomesLeader(func(context.Context, leadelect.LeaderElector) { became.Add(1) })
			le.RegisterOnLeaderLost(func(context.Context, leadelect.LeaderElector) { lost.Add(1) })

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			require.NoError(t, le.Start(ctx))
			testhelpers.WaitFor(t, 10*time.Second, func() bool { return became.Load() == 1 }, "the node did not become leader")

			cancel()
			tc.afterCancel(t, le, &lost)

			// The restart finishes the canceled run, so its loss has been
			// delivered by the time Start returns.
			require.NoError(t, le.Start(t.Context()))
			require.EqualValues(t, 1, lost.Load(), "the loss callback must run exactly once")
			testhelpers.WaitFor(t, 10*time.Second, func() bool { return became.Load() == 2 && le.IsLeader() },
				"the node did not become leader after the restart")
			require.True(t, le.IsRunning())

			// Stop ends the second term: one more loss, delivered before it
			// returns, and none after a repeated Stop.
			require.NoError(t, le.Stop(t.Context()))
			require.False(t, le.IsRunning())
			require.EqualValues(t, 2, lost.Load(), "Stop on a live leader must run the loss callback exactly once")
			require.NoError(t, le.Stop(t.Context()))
			require.EqualValues(t, 2, lost.Load())
		})
	}
}

// A provider whose Start context is canceled publishes the loss and the stop
// once, and Start restarts it without a Stop in between.
func TestProvider_StartAfterContextCancelRestarts(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	p, err := lenats.New(t.Context(), natstest.ConnectNATS(t, ns), lenats.WithBucket("restart-after-cancel"))
	require.NoError(t, err)

	lostCh := make(chan struct{}, 2)
	stopCh := make(chan struct{}, 2)
	cfg := providers.Config{Key: "k", NodeId: "n", TTL: 30 * time.Second, BecameCh: make(chan struct{}, 2), LostCh: lostCh, StopCh: stopCh}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, p.Start(ctx, cfg))
	testhelpers.WaitFor(t, 10*time.Second, p.IsLeader, "the node did not become leader")
	require.Error(t, p.Start(t.Context(), cfg), "a second Start while running must still be refused")

	cancel()
	require.NoError(t, p.Start(t.Context(), cfg), "Start must restart an election ended by its context")
	require.Len(t, lostCh, 1, "the canceled run must publish its loss exactly once")
	require.Len(t, stopCh, 1, "the canceled run must publish its stop exactly once")

	testhelpers.WaitFor(t, 10*time.Second, p.IsLeader, "the node did not become leader after the restart")
	require.True(t, p.IsRunning())

	require.NoError(t, p.Stop(t.Context()))
	require.Len(t, lostCh, 2, "Stop on a live leader must publish its loss exactly once")
	require.Len(t, stopCh, 2)
}
