// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNew_Defaults(t *testing.T) {
	p := New()
	require.NotNil(t, p, "New() returned nil")
	require.Equal(t, DefaultPoolSize, p.opts.size)
	require.Equal(t, DefaultMaxIdleTime, p.opts.maxIdleTime)
	require.Equal(t, DefaultCleanupInterval, p.opts.cleanupInterval)
	require.Equal(t, DefaultConnectTimeout, p.opts.connectTimeout)
}

func TestNew_WithOptions(t *testing.T) {
	p := New(
		WithSize(5),
		WithMaxIdleTime(10*time.Minute),
		WithCleanupInterval(2*time.Minute),
		WithConnectTimeout(5*time.Second),
	)
	require.Equal(t, 5, p.opts.size)
	require.Equal(t, 10*time.Minute, p.opts.maxIdleTime)
}

func TestWithSize_Zero(t *testing.T) {
	p := New(WithSize(0))
	require.Equal(t, 0, p.opts.size)
}

func TestWithMaxIdleTime_Negative(t *testing.T) {
	p := New(WithMaxIdleTime(-1))
	require.Equal(t, DefaultMaxIdleTime, p.opts.maxIdleTime)
}

func TestWithCleanupInterval_Negative(t *testing.T) {
	p := New(WithCleanupInterval(-1))
	require.Equal(t, DefaultCleanupInterval, p.opts.cleanupInterval)
}

func TestWithConnectTimeout_Negative(t *testing.T) {
	p := New(WithConnectTimeout(-1))
	require.Equal(t, DefaultConnectTimeout, p.opts.connectTimeout)
}

func TestStartAndStop(t *testing.T) {
	p := New(WithCleanupInterval(50 * time.Millisecond))
	ctx := t.Context()

	stop, err := p.Start(ctx)
	require.NoError(t, err)
	stop()
}

func TestStart_AfterStopped(t *testing.T) {
	p := New()
	ctx := t.Context()

	stop, err := p.Start(ctx)
	require.NoError(t, err)
	stop()

	_, err = p.Start(ctx)
	require.Equal(t, ErrConnectionPoolClosed, err)
}

func TestGetConnection_AfterStopped(t *testing.T) {
	p := New()
	ctx := t.Context()
	stop, _ := p.Start(ctx)
	stop()

	_, err := p.GetConnection(ctx, "localhost:9999")
	require.Equal(t, ErrConnectionPoolClosed, err)
}

func TestReturnConnection_Nil(t *testing.T) {
	p := New()
	// Should not panic
	p.ReturnConnection(nil)
}

func TestGetConnection_WithClientFactory(t *testing.T) {
	factoryCalled := false
	factory := func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		factoryCalled = true
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	p := New(WithClientFactory(factory), WithCleanupInterval(50*time.Millisecond))
	ctx := t.Context()

	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	require.True(t, factoryCalled, "client factory was not called")
	p.ReturnConnection(conn)
}

func TestGetConnection_DefaultFactory(t *testing.T) {
	p := New(WithCleanupInterval(50 * time.Millisecond))
	ctx := t.Context()

	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)
}

func TestReturnAndReuse(t *testing.T) {
	p := New(WithClientFactory(func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}), WithCleanupInterval(50*time.Millisecond))

	ctx := t.Context()

	stop, _ := p.Start(ctx)
	defer stop()

	conn1, _ := p.GetConnection(ctx, "localhost:0")
	p.ReturnConnection(conn1)

	conn2, _ := p.GetConnection(ctx, "localhost:0")
	p.ReturnConnection(conn2)

	// conn2 may or may not be same as conn1 depending on health check,
	// but neither call should error
}

func TestContextCancellation_StopsPool(t *testing.T) {
	p := New(WithCleanupInterval(50 * time.Millisecond))
	ctx, cancel := context.WithCancel(t.Context())

	stop, err := p.Start(ctx)
	require.NoError(t, err)

	cancel()
	// stop should not hang after context cancellation
	stop()
}

func TestDefaultConstants(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
	}{
		{"DefaultMaxIdleTime", DefaultMaxIdleTime},
		{"DefaultCleanupInterval", DefaultCleanupInterval},
		{"DefaultConnectTimeout", DefaultConnectTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, tt.got <= 0, "%s = %v", tt.name, tt.got)
		})
	}
	require.False(t, DefaultPoolSize <= 0, "DefaultPoolSize = %d", DefaultPoolSize)
}
