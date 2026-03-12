// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// uniqMetrics holds all Prometheus metrics for the Uniq service.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type uniqMetrics struct {
	operationsTotal   metrics.Counter
	operationDuration metrics.Timer
	operationErrors   metrics.Counter
}

func newUniqMetrics(c metrics.Collector) *uniqMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("uniq")

	return &uniqMetrics{
		operationsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "operations_total",
			Help:       "Total number of uniqueness operations performed.",
			LabelNames: []string{"op"},
		}),
		operationDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "operation_duration_seconds",
				Help:       "Duration of uniqueness operations in seconds.",
				LabelNames: []string{"op"},
			},
		}),
		operationErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "operation_errors_total",
			Help:       "Total number of uniqueness operation failures.",
			LabelNames: []string{"op"},
		}),
	}
}
