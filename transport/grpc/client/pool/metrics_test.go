// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
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
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	val := getCounterValue(t, registry, "test_grpc_connection_pool_connections_created_total",
		"target", "localhost:0")
	assert.Equal(t, float64(1), val, "connections_created_total should be 1")
}

func TestPool_Metrics_ConnectionsReused(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
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

	reused := getCounterValue(t, registry, "test_grpc_connection_pool_connections_reused_total",
		"target", "localhost:0")
	assert.GreaterOrEqual(t, reused, float64(1), "connections_reused_total should be >= 1")
}

func TestPool_Metrics_ActiveAndInUse(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)

	active := getGaugeValue(t, registry, "test_grpc_connection_pool_connections_active")
	assert.Equal(t, float64(1), active, "connections_active should be 1 while in use")

	inUse := getGaugeValue(t, registry, "test_grpc_connection_pool_connections_in_use")
	assert.Equal(t, float64(1), inUse, "connections_in_use should be 1 while in use")

	p.ReturnConnection(conn)

	inUse = getGaugeValue(t, registry, "test_grpc_connection_pool_connections_in_use")
	assert.Equal(t, float64(0), inUse, "connections_in_use should be 0 after return")

	idle := getGaugeValue(t, registry, "test_grpc_connection_pool_connections_idle")
	assert.Equal(t, float64(1), idle, "connections_idle should be 1 after return")
}

func TestPool_Metrics_ConnectDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	conn, err := p.GetConnection(ctx, "localhost:0")
	require.NoError(t, err)
	p.ReturnConnection(conn)

	count := getHistogramCount(t, registry, "test_grpc_connection_pool_connect_duration_seconds",
		"target", "localhost:0")
	assert.GreaterOrEqual(t, count, uint64(1), "connect_duration_seconds should have >= 1 observation")
}

func TestPool_Metrics_ConnectionsClosed(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithSize(2),
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
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

	closed := getCounterValue(t, registry, "test_grpc_connection_pool_connections_closed_total",
		"target", "localhost:0", "reason", "shutdown")
	assert.GreaterOrEqual(t, closed, float64(1), "connections_closed_total with reason=shutdown should be >= 1")
}

func TestPool_Metrics_CleanupDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
		WithClientFactory(testFactory),
	)

	ctx := t.Context()
	stop, err := p.Start(ctx)
	require.NoError(t, err)
	defer stop()

	// Wait for at least one cleanup cycle
	time.Sleep(100 * time.Millisecond)

	count := getHistogramCount(t, registry, "test_grpc_connection_pool_cleanup_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1), "cleanup_duration_seconds should have >= 1 observation")
}

func TestPool_Metrics_ConnectionErrors(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	p := New(
		WithCleanupInterval(50*time.Millisecond),
		WithCollector(collector),
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

	val := getCounterValue(t, registry, "test_grpc_connection_pool_connection_errors_total",
		"target", "localhost:0")
	assert.Equal(t, float64(1), val, "connection_errors_total should be 1")
}

// --- helpers ---

func testFactory(_ context.Context, target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func newTestCollector(registry *prometheus.Registry) metrics.Collector {
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	return metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))
}

func getGaugeValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func getCounterValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func getHistogramCount(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) uint64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

func gatherMetric(t *testing.T, registry *prometheus.Registry, name string) *promio.MetricFamily {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func findMetricByLabels(ms []*promio.Metric, labelPairs ...string) *promio.Metric {
	if len(labelPairs) == 0 {
		if len(ms) > 0 {
			return ms[0]
		}
		return nil
	}
	for _, m := range ms {
		if matchLabels(m.GetLabel(), labelPairs...) {
			return m
		}
	}
	return nil
}

func matchLabels(labels []*promio.LabelPair, pairs ...string) bool {
	for i := 0; i < len(pairs)-1; i += 2 {
		key, val := pairs[i], pairs[i+1]
		found := false
		for _, lp := range labels {
			if lp.GetName() == key && lp.GetValue() == val {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
