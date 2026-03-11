// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// keeperMetrics holds all Prometheus metrics for the Keeper.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type keeperMetrics struct {
	locksAcquired metrics.Counter
	locksDenied   metrics.Counter
	completions   metrics.Counter
	deletions     metrics.Counter
	errors        metrics.Counter
}

func newKeeperMetrics(c metrics.Collector) *keeperMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("idempotency")

	return &keeperMetrics{
		locksAcquired: scoped.MustCounter(metrics.MetricOpts{
			Name: "locks_acquired_total",
			Help: "Total number of idempotency keys successfully locked.",
		}),
		locksDenied: scoped.MustCounter(metrics.MetricOpts{
			Name: "locks_denied_total",
			Help: "Total number of duplicate requests detected.",
		}),
		completions: scoped.MustCounter(metrics.MetricOpts{
			Name: "completions_total",
			Help: "Total number of idempotency keys marked as complete.",
		}),
		deletions: scoped.MustCounter(metrics.MetricOpts{
			Name: "deletions_total",
			Help: "Total number of idempotency keys deleted.",
		}),
		errors: scoped.MustCounter(metrics.MetricOpts{
			Name: "errors_total",
			Help: "Total number of idempotency operation errors.",
		}),
	}
}
