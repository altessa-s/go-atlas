// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/observability/metrics"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// newPromCollector builds a Collector backed by a real Prometheus adapter on
// a private registry, so benchmarks measure the production recording path.
func newPromCollector(tb testing.TB) metrics.Collector {
	tb.Helper()
	adapter := prometheusadapter.New(
		prometheusadapter.WithRegisterer(prometheus.NewRegistry()),
	)
	return metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("bench"))
}

func BenchmarkLabeledCounterInc(b *testing.B) {
	c := newPromCollector(b)
	ctr := c.MustCounter(metrics.MetricOpts{
		Name:       "requests_total",
		Help:       "bench",
		LabelNames: []string{"method"},
	})
	labels := metrics.Labels{"method": "GET"}

	b.Run("prebound", func(b *testing.B) {
		bound := ctr.WithLabels(labels)
		b.ReportAllocs()
		for b.Loop() {
			bound.Inc()
		}
	})
	b.Run("per_call", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			ctr.WithLabels(labels).Inc()
		}
	})
}

func BenchmarkLabeledHistogramObserve(b *testing.B) {
	c := newPromCollector(b)
	h := c.MustHistogram(metrics.HistogramOpts{
		MetricOpts: metrics.MetricOpts{
			Name:       "request_size_bytes",
			Help:       "bench",
			LabelNames: []string{"method"},
		},
	})
	labels := metrics.Labels{"method": "GET"}

	b.Run("prebound", func(b *testing.B) {
		bound := h.WithLabels(labels)
		b.ReportAllocs()
		for b.Loop() {
			bound.Observe(0.42)
		}
	})
	b.Run("per_call", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			h.WithLabels(labels).Observe(0.42)
		}
	})
}

func BenchmarkLabeledTimerObserveDuration(b *testing.B) {
	c := newPromCollector(b)
	t := c.MustTimer(metrics.HistogramOpts{
		MetricOpts: metrics.MetricOpts{
			Name:       "request_duration_seconds",
			Help:       "bench",
			LabelNames: []string{"method"},
		},
	})
	bound := t.WithLabels(metrics.Labels{"method": "GET"})

	b.ReportAllocs()
	for b.Loop() {
		bound.ObserveDuration(42 * time.Millisecond)
	}
}

func BenchmarkLabeledGaugeSet(b *testing.B) {
	c := newPromCollector(b)
	g := c.MustGauge(metrics.MetricOpts{
		Name:       "in_flight",
		Help:       "bench",
		LabelNames: []string{"method"},
	})
	bound := g.WithLabels(metrics.Labels{"method": "GET"})

	b.ReportAllocs()
	for b.Loop() {
		bound.Set(42)
	}
}
