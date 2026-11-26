// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func countTargetPools(p *ConnectionPool) int {
	n := 0
	p.pools.Range(func(_, _ any) bool { n++; return true })
	return n
}

// TestFailedConnectionsKeepReconnectBackoff returns connections that failed to
// connect: the pool used to close them, so every borrower dialed a fresh
// connection and the dial rate followed the request rate.
func TestFailedConnectionsKeepReconnectBackoff(t *testing.T) {
	t.Parallel()

	var dials atomic.Int32
	p := New(WithSize(1), WithClientFactory(func(_ context.Context, target string) (*grpc.ClientConn, error) {
		dials.Add(1)
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return nil, errors.New("refused") }))
	}))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	const cycles = 5
	for range cycles {
		conn, err := p.GetConnection(t.Context(), "passthrough:///refused")
		require.NoError(t, err)
		conn.Connect()
		testhelpers.WaitFor(t, 5*time.Second, func() bool {
			s := conn.GetState()
			return s == connectivity.TransientFailure || s == connectivity.Connecting
		}, "connection did not start connecting")
		p.ReturnConnection(conn)
	}
	require.EqualValues(t, 1, dials.Load(), "connections still reconnecting must be reused")
}

// TestIdleTargetPoolsAreRetired creates target pools with a failing factory
// and runs cleanup: empty pools must not accumulate.
func TestIdleTargetPoolsAreRetired(t *testing.T) {
	t.Parallel()

	errDial := errors.New("dial refused")
	p := New(WithClientFactory(func(context.Context, string) (*grpc.ClientConn, error) { return nil, errDial }))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	for i := range 50 {
		_, err := p.GetConnection(t.Context(), fmt.Sprintf("backend-%d:443", i))
		require.ErrorIs(t, err, errDial)
	}
	require.Equal(t, 50, countTargetPools(p))

	p.cleanup()
	require.Zero(t, countTargetPools(p), "empty target pools must be retired")
}

// TestBorrowPrefersReadyConnection returns a Ready and a TransientFailure
// connection, the failing one last: the borrower must get the Ready one.
func TestBorrowPrefersReadyConnection(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	var calls atomic.Int32
	p := New(WithSize(2), WithClientFactory(func(_ context.Context, _ string) (*grpc.ClientConn, error) {
		if calls.Add(1) == 1 {
			return grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		return grpc.NewClient("passthrough:///refused", grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return nil, errors.New("refused") }))
	}))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	ready, err := p.GetConnection(t.Context(), "backend")
	require.NoError(t, err)
	failing, err := p.GetConnection(t.Context(), "backend")
	require.NoError(t, err)
	ready.Connect()
	failing.Connect()
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return ready.GetState() == connectivity.Ready }, "no Ready connection")
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return failing.GetState() == connectivity.TransientFailure }, "no failing connection")

	p.ReturnConnection(ready)
	p.ReturnConnection(failing)

	got, err := p.GetConnection(t.Context(), "backend")
	require.NoError(t, err)
	require.True(t, ready == got, "a TransientFailure connection was preferred over a Ready one")
}
