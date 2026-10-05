// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Lookup outcomes, used as the "result" label value of lookups_total.
const (
	lookupPositive = "positive" // the value might exist.
	lookupNegative = "negative" // the value definitely does not exist.
	lookupError    = "error"    // the lookup failed.
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

// observer returns an [Observer] that records into m with the filter_name
// label bound to name. Labeled handles are bound once, off the hot path.
func (m *probfilterMetrics) observer(name string) *metricsObserver {
	return &metricsObserver{
		positive:        m.lookupsTotal.WithLabels(metrics.Labels{"filter_name": name, "result": lookupPositive}),
		negative:        m.lookupsTotal.WithLabels(metrics.Labels{"filter_name": name, "result": lookupNegative}),
		failed:          m.lookupsTotal.WithLabels(metrics.Labels{"filter_name": name, "result": lookupError}),
		adds:            m.addsTotal.WithLabels(metrics.Labels{"filter_name": name}),
		lookupDuration:  m.lookupDuration.WithLabels(metrics.Labels{"filter_name": name}),
		rebuildDuration: m.rebuildDuration,
		rebuildErrors:   m.rebuildErrors,
	}
}

// metricsObserver is the [Observer] a [Manager] attaches to a registered
// [ObservableFilter].
type metricsObserver struct {
	positive        metrics.Counter
	negative        metrics.Counter
	failed          metrics.Counter
	adds            metrics.Counter
	lookupDuration  metrics.Timer
	rebuildDuration metrics.Timer
	rebuildErrors   metrics.Counter
}

var _ Observer = (*metricsObserver)(nil)

// ObserveLookup implements [Observer].
func (o *metricsObserver) ObserveLookup(found bool, err error, elapsed time.Duration) {
	switch {
	case err != nil:
		o.failed.Inc()
	case found:
		o.positive.Inc()
	default:
		o.negative.Inc()
	}
	o.lookupDuration.ObserveDuration(elapsed)
}

// ObserveAdd implements [Observer].
func (o *metricsObserver) ObserveAdd(n int) {
	if n > 0 {
		o.adds.Add(float64(n))
	}
}

// ObserveRebuild implements [Observer].
func (o *metricsObserver) ObserveRebuild(elapsed time.Duration, err error) {
	o.rebuildDuration.ObserveDuration(elapsed)
	if err != nil {
		o.rebuildErrors.Inc()
	}
}
