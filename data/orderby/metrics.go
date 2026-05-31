// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// orderByMetrics holds Prometheus metrics for the order_by parser. When
// no [metrics.Collector] is provided, [metrics.Noop] is used and all
// methods become zero-cost no-ops.
//
// Only the parser side is instrumented here. Translators live in
// independent subpackages and are deliberately not given the collector,
// so introducing a "translations_total" counter at this level would
// register a metric that is never incremented. When per-backend
// translation telemetry becomes necessary, plumb a dedicated collector
// through each translator's options rather than adding the field here.
type orderByMetrics struct {
	parseDuration metrics.Timer
	parseErrors   metrics.Counter
}

func newOrderByMetrics(c metrics.Collector) *orderByMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("orderby")

	return &orderByMetrics{
		parseDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "parse_duration_seconds",
				Help: "Duration of order_by parse operations in seconds.",
			},
		}),
		parseErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "parse_errors_total",
			Help: "Total number of order_by parse failures.",
		}),
	}
}
