// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestCapacityWaitsForReturnOrCancellation(t *testing.T) {
	t.Parallel()
	var created atomic.Int32
	p := pool.New(pool.WithSize(1), pool.WithClientFactory(func(_ context.Context, target string) (*grpc.ClientConn, error) {
		created.Add(1)
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	first, err := p.GetConnection(t.Context(), "localhost:1")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err = p.GetConnection(ctx, "localhost:1")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, int32(1), created.Load())
	p.ReturnConnection(first)
	second, err := p.GetConnection(t.Context(), "localhost:1")
	require.NoError(t, err)
	require.Same(t, first, second)
	p.ReturnConnection(second)
	// Returning twice must not create duplicate idle entries/accounting.
	p.ReturnConnection(second)
	third, err := p.GetConnection(t.Context(), "localhost:1")
	require.NoError(t, err)
	p.ReturnConnection(third)
}

func TestStopCancelsFactoriesAndClosesLateConnections(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	created := make(chan *grpc.ClientConn, 1)
	p := pool.New(pool.WithClientFactory(func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		close(entered)
		<-ctx.Done()
		conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		created <- conn
		return conn, err
	}))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	finished := make(chan error, 1)
	go func() { _, err := p.GetConnection(t.Context(), "localhost:1"); finished <- err }()
	<-entered
	stop()
	require.ErrorIs(t, <-finished, pool.ErrConnectionPoolClosed)
	conn := <-created
	require.NotNil(t, conn)
	require.Equal(t, connectivity.Shutdown, conn.GetState())
}

func TestBindingsKeepPoliciesSeparateAndShareCapacity(t *testing.T) {
	t.Parallel()
	p := pool.New(pool.WithSize(1))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	var callsA, callsB atomic.Int32
	factory := func(calls *atomic.Int32) pool.ClientFactory {
		return func(_ context.Context, target string) (*grpc.ClientConn, error) {
			calls.Add(1)
			return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
	}
	a, err := p.Bind("localhost:1", factory(&callsA))
	require.NoError(t, err)
	b, err := p.Bind("localhost:1", factory(&callsB))
	require.NoError(t, err)
	connA, err := a.GetConnection(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err = b.GetConnection(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, callsB.Load())
	p.ReturnConnection(connA)
	connB, err := b.GetConnection(t.Context())
	require.NoError(t, err)
	require.NotSame(t, connA, connB)
	require.Equal(t, connectivity.Shutdown, connA.GetState())
	require.Equal(t, int32(1), callsB.Load())
	p.ReturnConnection(connB)
}

func TestStartRejectsDuplicateLifecycle(t *testing.T) {
	t.Parallel()
	p := pool.New()
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)
	_, err = p.Start(t.Context())
	require.ErrorIs(t, err, pool.ErrAlreadyStarted)
}
