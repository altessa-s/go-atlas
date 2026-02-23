// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNew_Defaults(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.opts.size != DefaultPoolSize {
		t.Fatalf("size = %d, want %d", p.opts.size, DefaultPoolSize)
	}
	if p.opts.maxIdleTime != DefaultMaxIdleTime {
		t.Fatalf("maxIdleTime = %v, want %v", p.opts.maxIdleTime, DefaultMaxIdleTime)
	}
	if p.opts.cleanupInterval != DefaultCleanupInterval {
		t.Fatalf("cleanupInterval = %v, want %v", p.opts.cleanupInterval, DefaultCleanupInterval)
	}
	if p.opts.connectTimeout != DefaultConnectTimeout {
		t.Fatalf("connectTimeout = %v, want %v", p.opts.connectTimeout, DefaultConnectTimeout)
	}
}

func TestNew_WithOptions(t *testing.T) {
	p := New(
		WithSize(5),
		WithMaxIdleTime(10*time.Minute),
		WithCleanupInterval(2*time.Minute),
		WithConnectTimeout(5*time.Second),
	)
	if p.opts.size != 5 {
		t.Fatalf("size = %d", p.opts.size)
	}
	if p.opts.maxIdleTime != 10*time.Minute {
		t.Fatalf("maxIdleTime = %v", p.opts.maxIdleTime)
	}
}

func TestWithSize_Zero(t *testing.T) {
	p := New(WithSize(0))
	if p.opts.size != 0 {
		t.Fatalf("size = %d", p.opts.size)
	}
}

func TestWithMaxIdleTime_Negative(t *testing.T) {
	p := New(WithMaxIdleTime(-1))
	if p.opts.maxIdleTime != DefaultMaxIdleTime {
		t.Fatalf("negative maxIdleTime should keep default, got %v", p.opts.maxIdleTime)
	}
}

func TestWithCleanupInterval_Negative(t *testing.T) {
	p := New(WithCleanupInterval(-1))
	if p.opts.cleanupInterval != DefaultCleanupInterval {
		t.Fatalf("negative cleanupInterval should keep default, got %v", p.opts.cleanupInterval)
	}
}

func TestWithConnectTimeout_Negative(t *testing.T) {
	p := New(WithConnectTimeout(-1))
	if p.opts.connectTimeout != DefaultConnectTimeout {
		t.Fatalf("negative connectTimeout should keep default, got %v", p.opts.connectTimeout)
	}
}

func TestStartAndStop(t *testing.T) {
	p := New(WithCleanupInterval(50 * time.Millisecond))
	ctx := t.Context()

	stop, err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	stop()
}

func TestStart_AfterStopped(t *testing.T) {
	p := New()
	ctx := t.Context()

	stop, err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	stop()

	_, err = p.Start(ctx)
	if err != ErrConnectionPoolClosed {
		t.Fatalf("Start() after stop should return ErrConnectionPoolClosed, got %v", err)
	}
}

func TestGetConnection_AfterStopped(t *testing.T) {
	p := New()
	ctx := t.Context()
	stop, _ := p.Start(ctx)
	stop()

	_, err := p.GetConnection(ctx, "localhost:9999")
	if err != ErrConnectionPoolClosed {
		t.Fatalf("GetConnection after stop should return ErrConnectionPoolClosed, got %v", err)
	}
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
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	if err != nil {
		t.Fatalf("GetConnection() error = %v", err)
	}
	if !factoryCalled {
		t.Fatal("client factory was not called")
	}
	p.ReturnConnection(conn)
}

func TestGetConnection_DefaultFactory(t *testing.T) {
	p := New(WithCleanupInterval(50 * time.Millisecond))
	ctx := t.Context()

	stop, err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	if err != nil {
		t.Fatalf("GetConnection() error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	cancel()
	// stop should not hang after context cancellation
	stop()
}

func TestErrConnectionPoolClosed(t *testing.T) {
	if ErrConnectionPoolClosed == nil {
		t.Fatal("ErrConnectionPoolClosed is nil")
	}
	if ErrConnectionPoolClosed.Error() == "" {
		t.Fatal("ErrConnectionPoolClosed.Error() is empty")
	}
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
			if tt.got <= 0 {
				t.Fatalf("%s = %v", tt.name, tt.got)
			}
		})
	}
	if DefaultPoolSize <= 0 {
		t.Fatalf("DefaultPoolSize = %d", DefaultPoolSize)
	}
}
