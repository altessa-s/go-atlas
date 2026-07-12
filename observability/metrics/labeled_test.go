// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// newPromRegistryCollector builds a Collector backed by a Prometheus adapter
// on a private registry so tests can read recorded values back via Gather.
func newPromRegistryCollector(t *testing.T) (metrics.Collector, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	adapter := prometheusadapter.New(
		prometheusadapter.WithRegisterer(reg),
		prometheusadapter.WithGatherer(reg),
	)
	return metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("svc")), reg
}

// promValue returns (value, sampleCount) for the metric child matching the
// exact label set: counters and gauges report their value, histograms report
// (sample sum, sample count).
func promValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) (float64, uint64) {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			got := make(map[string]string, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				got[lp.GetName()] = lp.GetValue()
			}
			if len(got) != len(labels) {
				continue
			}
			match := true
			for k, v := range labels {
				if got[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			if h := m.GetHistogram(); h != nil {
				return h.GetSampleSum(), h.GetSampleCount()
			}
			if c := m.GetCounter(); c != nil {
				return c.GetValue(), 0
			}
			if g := m.GetGauge(); g != nil {
				return g.GetValue(), 0
			}
		}
	}
	return 0, 0
}

func TestWithLabels_PrometheusBound_CounterRecords(t *testing.T) {
	t.Parallel()
	c, reg := newPromRegistryCollector(t)
	ctr := c.MustCounter(metrics.MetricOpts{
		Name:       "hits_total",
		Help:       "test",
		LabelNames: []string{"method"},
	})

	get := ctr.WithLabels(metrics.Labels{"method": "GET"})
	get.Inc()
	get.Add(2)
	ctr.WithLabels(metrics.Labels{"method": "POST"}).Inc()

	v, _ := promValue(t, reg, "svc_hits_total", map[string]string{"method": "GET"})
	require.Equal(t, float64(3), v)
	v, _ = promValue(t, reg, "svc_hits_total", map[string]string{"method": "POST"})
	require.Equal(t, float64(1), v)
}

func TestWithLabels_PrometheusBound_ChainedMerge(t *testing.T) {
	t.Parallel()
	c, reg := newPromRegistryCollector(t)
	ctr := c.MustCounter(metrics.MetricOpts{
		Name:       "chained_total",
		Help:       "test",
		LabelNames: []string{"a", "b"},
	})

	ctr.WithLabels(metrics.Labels{"a": "1"}).WithLabels(metrics.Labels{"b": "2"}).Inc()

	v, _ := promValue(t, reg, "svc_chained_total", map[string]string{"a": "1", "b": "2"})
	require.Equal(t, float64(1), v)
}

func TestWithLabels_PrometheusBound_GaugeAndTimer(t *testing.T) {
	t.Parallel()
	c, reg := newPromRegistryCollector(t)

	g := c.MustGauge(metrics.MetricOpts{Name: "in_flight", Help: "test", LabelNames: []string{"method"}})
	bg := g.WithLabels(metrics.Labels{"method": "GET"})
	bg.Inc()
	bg.Inc()
	bg.Dec()
	v, _ := promValue(t, reg, "svc_in_flight", map[string]string{"method": "GET"})
	require.Equal(t, float64(1), v)

	tm := c.MustTimer(metrics.HistogramOpts{
		MetricOpts: metrics.MetricOpts{Name: "latency_seconds", Help: "test", LabelNames: []string{"method"}},
	})
	tm.WithLabels(metrics.Labels{"method": "GET"}).ObserveDuration(250 * time.Millisecond)
	sum, count := promValue(t, reg, "svc_latency_seconds", map[string]string{"method": "GET"})
	require.Equal(t, uint64(1), count)
	require.InDelta(t, 0.25, sum, 1e-9)
}

// TestWithLabels_FallbackWithoutBinder pins the legacy path: adapters that do
// not implement adapters.Binder (like the in-memory one) must keep recording
// through RecordCounter with the stored label map.
func TestWithLabels_FallbackWithoutBinder(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	ctr := tc.MustCounter(metrics.MetricOpts{
		Name:       "fallback_total",
		Help:       "test",
		LabelNames: []string{"method"},
	})

	ctr.WithLabels(metrics.Labels{"method": "GET"}).Inc()

	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, tc, "test_fallback_total", "method", "GET"))
}

func TestWithLabels_BoundCounterInc_NoAllocs(t *testing.T) {
	c, _ := newPromRegistryCollector(t)
	ctr := c.MustCounter(metrics.MetricOpts{
		Name:       "alloc_guard_total",
		Help:       "test",
		LabelNames: []string{"method"},
	})
	bound := ctr.WithLabels(metrics.Labels{"method": "GET"})

	allocs := testing.AllocsPerRun(100, func() {
		bound.Inc()
	})
	require.Zero(t, allocs, "bound counter Inc must not allocate")
}
