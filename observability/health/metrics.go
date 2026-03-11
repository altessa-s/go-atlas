// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// healthMetrics holds all Prometheus metrics for the health Coordinator.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type healthMetrics struct {
	checkCycleDuration metrics.Timer
	statusChanges      metrics.Counter
	checksPerformed    metrics.Counter
}

func newHealthMetrics(c metrics.Collector) *healthMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("health")

	return &healthMetrics{
		checkCycleDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "check_cycle_duration_seconds",
				Help: "Duration of a single health check cycle in seconds.",
			},
		}),
		statusChanges: scoped.MustCounter(metrics.MetricOpts{
			Name: "status_changes_total",
			Help: "Total number of health status changes detected.",
		}),
		checksPerformed: scoped.MustCounter(metrics.MetricOpts{
			Name: "checks_performed_total",
			Help: "Total number of individual health checks performed.",
		}),
	}
}
