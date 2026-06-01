// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// filterMetrics holds Prometheus metrics for the CEL filter parser. When
// no [metrics.Collector] is provided, [metrics.Noop] is used and all
// methods become zero-cost no-ops.
//
// Only the parser side is instrumented here. Translators live in
// independent subpackages and are deliberately not given the collector,
// so introducing a "translations_total" counter at this level would
// register a metric that is never incremented. When per-backend
// translation telemetry becomes necessary, plumb a dedicated collector
// through each translator's options rather than adding the field here.
type filterMetrics struct {
	parseDuration metrics.Timer
	parseErrors   metrics.Counter
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
	}
}
