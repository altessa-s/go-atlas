// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// auditMetrics holds all Prometheus metrics for the audit dispatcher.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type auditMetrics struct {
	eventsEmitted  metrics.Counter
	eventsDropped  metrics.Counter
	flushDuration  metrics.Timer
	storeErrors    metrics.Counter
	workersActive  metrics.Gauge
}

func newAuditMetrics(c metrics.Collector) *auditMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("audit")

	return &auditMetrics{
		eventsEmitted: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_emitted_total",
			Help: "Total number of audit events successfully emitted.",
		}),
		eventsDropped: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_dropped_total",
			Help: "Total number of audit events dropped due to a full buffer.",
		}),
		flushDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "batch_flush_duration_seconds",
				Help: "Duration of batch flush operations in seconds.",
			},
		}),
		storeErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "store_errors_total",
			Help: "Total number of batch store failures after all retries.",
		}),
		workersActive: scoped.MustGauge(metrics.MetricOpts{
			Name: "workers_active",
			Help: "Number of currently active dispatch workers.",
		}),
	}
}
