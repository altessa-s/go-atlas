// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

func newAdapter(t *testing.T) (*prometheusadapter.Adapter, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	return prometheusadapter.New(
		prometheusadapter.WithRegisterer(reg),
		prometheusadapter.WithGatherer(reg),
	), reg
}

func register(t *testing.T, a *prometheusadapter.Adapter, name string, typ adapters.MetricType) {
	t.Helper()
	require.NoError(t, a.Register(&adapters.Desc{
		Name:       name,
		Help:       "test",
		Type:       typ,
		LabelNames: []string{"method"},
	}))
}

// counterValue extracts the counter child value for the given method label.
func counterValue(t *testing.T, reg *prometheus.Registry, name, method string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "method" && lp.GetValue() == method {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func TestRecordCounter(t *testing.T) {
	t.Parallel()
	a, reg := newAdapter(t)
	register(t, a, "requests_total", adapters.TypeCounter)

	a.RecordCounter("requests_total", map[string]string{"method": "GET"}, 1)
	a.RecordCounter("requests_total", map[string]string{"method": "GET"}, 2)
	a.RecordCounter("unknown_total", map[string]string{"method": "GET"}, 5) // silently dropped

	require.Equal(t, float64(3), counterValue(t, reg, "requests_total", "GET"))
}

func TestRegister_Idempotent(t *testing.T) {
	t.Parallel()
	a, _ := newAdapter(t)
	register(t, a, "requests_total", adapters.TypeCounter)
	register(t, a, "requests_total", adapters.TypeCounter)
}

func TestBindCounter(t *testing.T) {
	t.Parallel()
	a, reg := newAdapter(t)
	register(t, a, "bound_total", adapters.TypeCounter)

	bound, ok := a.BindCounter("bound_total", map[string]string{"method": "GET"})
	require.True(t, ok)
	bound.Add(1)
	bound.Add(2)

	require.Equal(t, float64(3), counterValue(t, reg, "bound_total", "GET"))
}

func TestBindCounter_UnknownMetric(t *testing.T) {
	t.Parallel()
	a, _ := newAdapter(t)
	_, ok := a.BindCounter("missing_total", map[string]string{"method": "GET"})
	require.False(t, ok)
}

func TestBindCounter_InvalidLabels(t *testing.T) {
	t.Parallel()
	a, _ := newAdapter(t)
	register(t, a, "labels_total", adapters.TypeCounter)
	_, ok := a.BindCounter("labels_total", map[string]string{"wrong": "x"})
	require.False(t, ok)
}

func TestBindGaugeAndHistogram(t *testing.T) {
	t.Parallel()
	a, reg := newAdapter(t)
	register(t, a, "in_flight", adapters.TypeGauge)
	register(t, a, "latency_seconds", adapters.TypeHistogram)

	g, ok := a.BindGauge("in_flight", map[string]string{"method": "GET"})
	require.True(t, ok)
	g.Set(7)

	h, ok := a.BindHistogram("latency_seconds", map[string]string{"method": "GET"})
	require.True(t, ok)
	h.Observe(0.5)

	mfs, err := reg.Gather()
	require.NoError(t, err)
	var sawGauge, sawHisto bool
	for _, mf := range mfs {
		switch mf.GetName() {
		case "in_flight":
			sawGauge = true
			require.Equal(t, float64(7), mf.GetMetric()[0].GetGauge().GetValue())
		case "latency_seconds":
			sawHisto = true
			require.Equal(t, uint64(1), mf.GetMetric()[0].GetHistogram().GetSampleCount())
		}
	}
	require.True(t, sawGauge)
	require.True(t, sawHisto)
}
