// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// engineMetrics holds the Prometheus metrics for an Engine.
// When no metrics.Collector is provided, metrics.Noop is used and all
// methods become zero-cost no-ops.
type engineMetrics struct {
	enqueued      metrics.Counter
	dropped       metrics.Counter
	flushDuration metrics.Timer
	sinkErrors    metrics.Counter
	walErrors     metrics.Counter
	encodeErrors  metrics.Counter
	workersActive metrics.Gauge
	walBytes      metrics.Gauge
	replayCount   metrics.Counter
}

func newEngineMetrics(c metrics.Collector, subsystem string) *engineMetrics {
	if c == nil {
		c = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}
	scoped := c.WithSubsystem(subsystem)
	return &engineMetrics{
		enqueued: scoped.MustCounter(metrics.MetricOpts{
			Name: "items_enqueued_total",
			Help: "Total items successfully enqueued for async dispatch.",
		}),
		dropped: scoped.MustCounter(metrics.MetricOpts{
			Name: "items_dropped_total",
			Help: "Total items dropped due to a full buffer.",
		}),
		flushDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "batch_flush_duration_seconds",
				Help: "Duration of batch sink store operations.",
			},
		}),
		sinkErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "sink_errors_total",
			Help: "Total batch store failures after all retries.",
		}),
		walErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "wal_errors_total",
			Help: "Total WAL append errors.",
		}),
		encodeErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "encode_errors_total",
			Help: "Total codec encode failures.",
		}),
		workersActive: scoped.MustGauge(metrics.MetricOpts{
			Name: "workers_active",
			Help: "Number of currently active dispatch workers.",
		}),
		walBytes: scoped.MustGauge(metrics.MetricOpts{
			Name: "wal_bytes",
			Help: "Total bytes currently in WAL across all segments.",
		}),
		replayCount: scoped.MustCounter(metrics.MetricOpts{
			Name: "wal_replay_total",
			Help: "Total records replayed from WAL on startup.",
		}),
	}
}
