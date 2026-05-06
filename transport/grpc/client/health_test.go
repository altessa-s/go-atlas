// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc/connectivity"
)

const (
	// unreachableTarget is a reserved port that refuses connections
	// instantly, so a *grpc.ClientConn pointed at it lands in
	// TransientFailure shortly after Connect.
	unreachableTarget = "127.0.0.1:1"
)

func TestNewClientHealthDisabledWithoutCoordinator(t *testing.T) {
	t.Parallel()
	c := &Client{address: unreachableTarget, options: *newOptions()}
	require.Nil(t, newClientHealth(c))
}

func TestClientCheckHealthSingleMode(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithHealthCoordinator(coord),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	require.NotNil(t, c.health)
	require.Equal(t, []string{DefaultClientHealthServiceName},
		slices.Sorted(slices.Values(slices.Collect(coord.ListServices()))))

	// Idle conn maps to Serving by default.
	require.Equal(t, health.StatusServing, c.health.CheckHealth(t.Context()))
}

func TestClientPushNotifySingleMode(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithHealthCoordinator(coord),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	sub, err := coord.Subscribe(t.Context(), DefaultClientHealthServiceName)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	c.conn.Connect()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case status := <-sub.Updates():
			if status == health.StatusDegraded {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe Degraded push within 5s, current state=%v", c.conn.GetState())
		}
	}
}

func TestClientCheckHealthPoolMode(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := pool.New(pool.WithCleanupInterval(50 * time.Millisecond))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithPool(p),
		WithHealthCoordinator(coord),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	// Pool mode: CheckHealth derives from pool.StateForTarget. With no
	// conn yet borrowed for this target the pool reports Idle, which the
	// default mapper treats as Serving.
	require.Equal(t, health.StatusServing, c.health.CheckHealth(t.Context()))
}

func TestClientPushNotifyPoolMode(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	p := pool.New(pool.WithCleanupInterval(50 * time.Millisecond))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithPool(p),
		WithHealthCoordinator(coord),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	sub, err := coord.Subscribe(t.Context(), DefaultClientHealthServiceName)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	// Borrow a conn from the pool to trigger the watcher attach.
	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { c.ReturnConnection(conn) })
	conn.Connect()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case status := <-sub.Updates():
			if status == health.StatusDegraded {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe Degraded push within 5s in pool mode")
		}
	}
}

func TestWithClientHealthStateMapperOverride(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	custom := func(connectivity.State) health.ServingStatus { return health.StatusDegraded }

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithHealthCoordinator(coord),
		WithHealthStateMapper(custom),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	require.Equal(t, health.StatusDegraded, c.health.CheckHealth(t.Context()))
}

func TestClientCloseStopsWatcher(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	c, err := New(t.Context(), unreachableTarget,
		WithInsecure(),
		WithHealthCoordinator(coord),
	)
	require.NoError(t, err)

	// Close should return cleanly and the goroutine wait should not hang.
	require.NoError(t, c.Close(t.Context()))
	// Subsequent close on the helper is a noop.
	c.health.detach()
}
