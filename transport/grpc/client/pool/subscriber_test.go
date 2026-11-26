// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// refusedTarget is a reserved port that refuses connections at once, so a
// connection to it reaches TransientFailure shortly after Connect.
const refusedTarget = "127.0.0.1:1"

func dialRefused(_ context.Context, target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// TestSubscriberMayStopThePool runs the stop function from a subscriber on
// its first notification. The tracker used to deliver on the watcher
// goroutine and shutdown waits for every watcher, so the call never returned.
func TestSubscriberMayStopThePool(t *testing.T) {
	t.Parallel()

	p := pool.New(pool.WithClientFactory(dialRefused))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)

	stopped := make(chan struct{})
	unsub, err := p.SubscribeTarget(refusedTarget, func(connectivity.State) {
		stop()
		close(stopped)
	})
	require.NoError(t, err)
	t.Cleanup(unsub)

	conn, err := p.GetConnection(t.Context(), refusedTarget)
	require.NoError(t, err)
	p.ReturnConnection(conn)
	// A stuck stop would block cleanup too, so it is not registered.

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "stop called from a subscriber did not return")
	}
}

// TestStateForTargetWithoutWatchers forces a connection into TransientFailure
// with no subscriber or health reporter. Without a watcher the tracked state
// used to stay Idle, which health maps to Serving.
func TestStateForTargetWithoutWatchers(t *testing.T) {
	t.Parallel()

	// A pool without a reporter runs no watcher.
	p := pool.New(pool.WithClientFactory(dialRefused))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	conn, err := p.GetConnection(t.Context(), refusedTarget)
	require.NoError(t, err)
	conn.Connect()
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		return conn.GetState() == connectivity.TransientFailure
	}, "connection did not reach TransientFailure")

	require.Equal(t, conn.GetState(), p.StateForTarget(refusedTarget))
	require.Equal(t, health.StatusDegraded, p.CheckHealth(t.Context()))
}
