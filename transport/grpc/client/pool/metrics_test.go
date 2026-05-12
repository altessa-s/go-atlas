// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestPool_Metrics_Noop(t *testing.T) {
	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	// Should not panic with noop metrics
}

func TestPool_Metrics_ConnectionsCreated(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	val := testhelpers.GetCounterValue(t, tc, "test_grpc_connection_pool_connections_created_total",
		"target", "localhost:0")
	assert.Equal(t, float64(1), val, "connections_created_total should be 1")
}

func TestPool_Metrics_ConnectionsReused(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	// Get again — should reuse the returned connection
	conn2, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn2)

	reused := testhelpers.GetCounterValue(t, tc, "test_grpc_connection_pool_connections_reused_total",
		"target", "localhost:0")
	assert.GreaterOrEqual(t, reused, float64(1), "connections_reused_total should be >= 1")
}

func TestPool_Metrics_ActiveAndInUse(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)

	active := testhelpers.GetGaugeValue(t, tc, "test_grpc_connection_pool_connections_active")
	assert.Equal(t, float64(1), active, "connections_active should be 1 while in use")

	inUse := testhelpers.GetGaugeValue(t, tc, "test_grpc_connection_pool_connections_in_use")
	assert.Equal(t, float64(1), inUse, "connections_in_use should be 1 while in use")

	p.ReturnConnection(conn)

	inUse = testhelpers.GetGaugeValue(t, tc, "test_grpc_connection_pool_connections_in_use")
	assert.Equal(t, float64(0), inUse, "connections_in_use should be 0 after return")

	idle := testhelpers.GetGaugeValue(t, tc, "test_grpc_connection_pool_connections_idle")
	assert.Equal(t, float64(1), idle, "connections_idle should be 1 after return")
}

func TestPool_Metrics_ConnectDuration(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	count := testhelpers.GetHistogramCount(t, tc, "test_grpc_connection_pool_connect_duration_seconds",
		"target", "localhost:0")
	assert.GreaterOrEqual(t, count, uint64(1), "connect_duration_seconds should have >= 1 observation")
}

func TestPool_Metrics_ConnectionsClosed(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithSize(2),
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	// Shutdown closes connections
	stop()

	closed := testhelpers.GetCounterValue(t, tc, "test_grpc_connection_pool_connections_closed_total",
		"target", "localhost:0", "reason", "shutdown")
	assert.GreaterOrEqual(t, closed, float64(1), "connections_closed_total with reason=shutdown should be >= 1")
}

func TestPool_Metrics_CleanupDuration(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	// Wait for at least one cleanup cycle
	time.Sleep(100 * time.Millisecond)

	count := testhelpers.GetHistogramCount(t, tc, "test_grpc_connection_pool_cleanup_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1), "cleanup_duration_seconds should have >= 1 observation")
}

func TestPool_Metrics_ConnectionErrors(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithClientFactory(func(_ context.Context, _ string) (*grpc.ClientConn, error) {
			return nil, assert.AnError
		}),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	_, err = p.GetConnection(ctx, "localhost:0")
	require.Error(t, err)

	val := testhelpers.GetCounterValue(t, tc, "test_grpc_connection_pool_connection_errors_total",
		"target", "localhost:0")
	assert.Equal(t, float64(1), val, "connection_errors_total should be 1")
}

// TestPool_MetricsSubsystem_CustomNamespace confirms that WithMetricsSubsystem
// scopes pool metrics under the caller's chosen subsystem instead of the
// default "grpc_connection_pool".
func TestPool_MetricsSubsystem_CustomNamespace(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(tc),
		WithMetricsSubsystem("egrul"),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	val := testhelpers.GetCounterValue(t, tc, "test_egrul_connections_created_total",
		"target", "localhost:0")
	assert.Equal(t, float64(1), val, "connections_created_total should be scoped under egrul")

	require.False(t, testhelpers.GatherMetric(t, tc, "test_grpc_connection_pool_connections_created_total"),
		"default subsystem must not appear when a custom one is set")
}

// TestPool_MetricsSubsystem_TwoPoolsShareRegistry pins the contract that two
// pools with distinct subsystems can share the same Prometheus registry
// without panicking on duplicate metric registration.
func TestPool_MetricsSubsystem_TwoPoolsShareRegistry(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	require.NotPanics(t, func() {
		egrul := New(
			WithCleanupInterval(50*time.Millisecond),
			WithCollector(tc),
			WithMetricsSubsystem("egrul"),
			WithClientFactory(testFactory),
		)
		kfocus := New(
			WithCleanupInterval(50*time.Millisecond),
			WithCollector(tc),
			WithMetricsSubsystem("kfocus"),
			WithClientFactory(testFactory),
		)

		ctx := t.Context()
		stopE, err := egrul.Start(ctx)
		require.NoError(t, err)
		defer stopE()
		stopK, err := kfocus.Start(ctx)
		require.NoError(t, err)
		defer stopK()

		connE, err := egrul.GetConnection(ctx, "localhost:0")
		require.NoError(t, err)
		egrul.ReturnConnection(connE)

		connK, err := kfocus.GetConnection(ctx, "localhost:0")
		require.NoError(t, err)
		kfocus.ReturnConnection(connK)
	})

	assert.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, tc, "test_egrul_connections_created_total",
			"target", "localhost:0"))
	assert.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, tc, "test_kfocus_connections_created_total",
			"target", "localhost:0"))
}

// --- helpers ---

func testFactory(_ context.Context, target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
