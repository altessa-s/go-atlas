// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
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

const refusedTarget = "passthrough:///refused"

// dialRefused returns connections whose every connection attempt fails at
// once, so they reach TransientFailure shortly after Connect without any
// dependence on what listens locally.
func dialRefused(_ context.Context, target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return nil, errors.New("refused") }),
	)
}

const stopChildEnv = "POOL_SUBSCRIBER_STOP_CHILD"

// TestSubscriberMayStopThePool runs the scenario in a subprocess with a
// deadline: the tracker used to deliver on the watcher goroutine while
// shutdown waits for every watcher, so a stop from a subscriber never
// returned. A hung child is killed instead of leaking goroutines here.
func TestSubscriberMayStopThePool(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSubscriberStopChild$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), stopChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "stop called from a subscriber did not return:\n%s", out)
}

// TestSubscriberStopChild is the body of the subprocess.
func TestSubscriberStopChild(t *testing.T) {
	if os.Getenv(stopChildEnv) == "" {
		t.Skip("runs as a subprocess of TestSubscriberMayStopThePool")
	}

	p := pool.New(pool.WithClientFactory(dialRefused))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)

	stopped := make(chan struct{})
	_, err = p.SubscribeTarget(refusedTarget, func(connectivity.State) {
		stop()
		close(stopped)
	})
	require.NoError(t, err)

	conn, err := p.GetConnection(t.Context(), refusedTarget)
	require.NoError(t, err)
	p.ReturnConnection(conn)

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
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
		return p.StateForTarget(refusedTarget) == connectivity.TransientFailure
	}, "tracked state did not follow the connection into TransientFailure")

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		return p.CheckHealth(t.Context()) == health.StatusDegraded
	}, "health did not report Degraded")
}
