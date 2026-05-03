// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the Prometheus subsystem name used when no
// explicit subsystem is provided via [WithMetricsSubsystem]. Override it
// when several pools target different upstreams from the same process so
// each pools metrics land in their own namespace and do not collide when
// registered against a shared [prometheus.Registerer].
const DefaultMetricsSubsystem = "grpc_connection_pool"

// poolMetrics holds all Prometheus metrics for the gRPC connection pool.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type poolMetrics struct {
	connectionsCreated metrics.Counter
	connectionsClosed  metrics.Counter
	connectionsReused  metrics.Counter
	connectionErrors   metrics.Counter
	connectionsActive  metrics.Gauge
	connectionsInUse   metrics.Gauge
	connectionsIdle    metrics.Gauge
	waiters            metrics.Gauge
	connectDuration    metrics.Timer
	cleanupDuration    metrics.Timer
	cleanupRemoved     metrics.Counter
}

func newPoolMetrics(c metrics.Collector, subsystem string) *poolMetrics {
	if c == nil {
		c = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}

	scoped := c.WithSubsystem(subsystem)

	return &poolMetrics{
		connectionsCreated: scoped.MustCounter(metrics.MetricOpts{
			Name:       "connections_created_total",
			Help:       "Total number of connections created.",
			LabelNames: []string{"target"},
		}),
		connectionsClosed: scoped.MustCounter(metrics.MetricOpts{
			Name:       "connections_closed_total",
			Help:       "Total number of connections closed.",
			LabelNames: []string{"target", "reason"},
		}),
		connectionsReused: scoped.MustCounter(metrics.MetricOpts{
			Name:       "connections_reused_total",
			Help:       "Total number of connections reused from the pool.",
			LabelNames: []string{"target"},
		}),
		connectionErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "connection_errors_total",
			Help:       "Total number of connection creation failures.",
			LabelNames: []string{"target"},
		}),
		connectionsActive: scoped.MustGauge(metrics.MetricOpts{
			Name: "connections_active",
			Help: "Number of currently active connections.",
		}),
		connectionsInUse: scoped.MustGauge(metrics.MetricOpts{
			Name: "connections_in_use",
			Help: "Number of connections currently in use by callers.",
		}),
		connectionsIdle: scoped.MustGauge(metrics.MetricOpts{
			Name: "connections_idle",
			Help: "Number of idle connections available in the pool.",
		}),
		waiters: scoped.MustGauge(metrics.MetricOpts{
			Name: "waiters",
			Help: "Number of goroutines waiting for a connection.",
		}),
		connectDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "connect_duration_seconds",
				Help:       "Duration of connection establishment in seconds.",
				LabelNames: []string{"target"},
			},
		}),
		cleanupDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "cleanup_duration_seconds",
				Help: "Duration of cleanup cycles in seconds.",
			},
		}),
		cleanupRemoved: scoped.MustCounter(metrics.MetricOpts{
			Name: "cleanup_connections_removed",
			Help: "Total number of connections removed during cleanup.",
		}),
	}
}
