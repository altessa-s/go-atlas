// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// projectionMetrics holds Prometheus metrics for the fields parser. When no
// [metrics.Collector] is provided, [metrics.Noop] is used and all methods
// become zero-cost no-ops. Translators are not instrumented, for the same
// reason as in data/orderby: they live in independent subpackages that do
// not receive the collector.
type projectionMetrics struct {
	parseDuration metrics.Timer
	parseErrors   metrics.Counter
}

func newProjectionMetrics(c metrics.Collector) *projectionMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("projection")

	return &projectionMetrics{
		parseDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "parse_duration_seconds",
				Help: "Duration of fields parse operations in seconds.",
			},
		}),
		parseErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "parse_errors_total",
			Help: "Total number of fields parse failures.",
		}),
	}
}
