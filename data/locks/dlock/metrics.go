// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// dlockMetrics holds all Prometheus metrics for DLock.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type dlockMetrics struct {
	locksAcquired    metrics.Counter
	locksReleased    metrics.Counter
	locksFailed      metrics.Counter
	acquireDuration  metrics.Timer
	synchronizations metrics.Counter
}

func newDlockMetrics(c metrics.Collector) *dlockMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("dlock")

	return &dlockMetrics{
		locksAcquired: scoped.MustCounter(metrics.MetricOpts{
			Name: "locks_acquired_total",
			Help: "Total number of locks successfully acquired.",
		}),
		locksReleased: scoped.MustCounter(metrics.MetricOpts{
			Name: "locks_released_total",
			Help: "Total number of locks successfully released. Pair with locks_acquired_total to detect leaks.",
		}),
		locksFailed: scoped.MustCounter(metrics.MetricOpts{
			Name: "locks_failed_total",
			Help: "Total number of lock acquisition failures.",
		}),
		acquireDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "acquire_duration_seconds",
				Help: "Duration of lock acquisition in seconds.",
			},
		}),
		synchronizations: scoped.MustCounter(metrics.MetricOpts{
			Name: "synchronizations_total",
			Help: "Total number of Synchronize calls completed.",
		}),
	}
}
