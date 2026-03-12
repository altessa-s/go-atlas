// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// probfilterMetrics holds all Prometheus metrics for the probabilistic filter manager.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type probfilterMetrics struct {
	lookupsTotal    metrics.Counter
	addsTotal       metrics.Counter
	lookupDuration  metrics.Timer
	rebuildDuration metrics.Timer
	rebuildErrors   metrics.Counter
}

func newProbfilterMetrics(c metrics.Collector) *probfilterMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("probfilter")

	return &probfilterMetrics{
		lookupsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "lookups_total",
			Help:       "Total number of probabilistic filter lookups.",
			LabelNames: []string{"filter_name", "result"},
		}),
		addsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "adds_total",
			Help:       "Total number of items added to probabilistic filters.",
			LabelNames: []string{"filter_name"},
		}),
		lookupDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "lookup_duration_seconds",
				Help:       "Duration of probabilistic filter lookup operations in seconds.",
				LabelNames: []string{"filter_name"},
			},
		}),
		rebuildDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "rebuild_duration_seconds",
				Help: "Duration of probabilistic filter rebuild operations in seconds.",
			},
		}),
		rebuildErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "rebuild_errors_total",
			Help: "Total number of failed probabilistic filter rebuild operations.",
		}),
	}
}
