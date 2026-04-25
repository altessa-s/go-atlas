// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// filterMetrics holds all Prometheus metrics for the CEL filter engine.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type filterMetrics struct {
	parseDuration     metrics.Timer
	parseErrors       metrics.Counter
	translationsTotal metrics.Counter
}

func newFilterMetrics(c metrics.Collector) *filterMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("filter")

	return &filterMetrics{
		parseDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "parse_duration_seconds",
				Help: "Duration of CEL expression parse operations in seconds.",
			},
		}),
		parseErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "parse_errors_total",
			Help: "Total number of CEL expression parse failures.",
		}),
		translationsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "translations_total",
			Help:       "Total number of filter translations performed.",
			LabelNames: []string{"target_backend"},
		}),
	}
}
