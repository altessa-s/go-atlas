// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestOpaMetrics_Noop(t *testing.T) {
	m := newOpaMetrics(nil)

	// Should not panic with noop metrics
	m.policyReloads.WithLabels(metrics.Labels{"result": "success"}).Inc()
	stop := m.reloadDuration.Start()
	stop()
	m.evaluations.WithLabels(metrics.Labels{"result": "allow"}).Inc()
	stop = m.evaluationDuration.Start()
	stop()
	m.modulesLoaded.Set(5)
}

func TestOpaMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)
	m := newOpaMetrics(collector)

	m.policyReloads.WithLabels(metrics.Labels{"result": "success"}).Inc()
	m.policyReloads.WithLabels(metrics.Labels{"result": "unchanged"}).Inc()
	stop := m.reloadDuration.Start()
	stop()
	m.evaluations.WithLabels(metrics.Labels{"result": "allow"}).Inc()
	m.evaluations.WithLabels(metrics.Labels{"result": "deny"}).Inc()
	stop = m.evaluationDuration.Start()
	stop()
	m.modulesLoaded.Set(3)

	val := getCounterValue(t, registry, "test_opa_policy_reloads_total", "result", "success")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_opa_policy_reloads_total", "result", "unchanged")
	assert.Equal(t, float64(1), val)

	count := getHistogramCount(t, registry, "test_opa_policy_reload_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	val = getCounterValue(t, registry, "test_opa_evaluations_total", "result", "allow")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_opa_evaluations_total", "result", "deny")
	assert.Equal(t, float64(1), val)

	count = getHistogramCount(t, registry, "test_opa_evaluation_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	gauge := getGaugeValue(t, registry, "test_opa_modules_loaded")
	assert.Equal(t, float64(3), gauge)
}

// --- helpers ---

func newTestCollector(registry *prometheus.Registry) metrics.Collector {
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	return metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))
}

func getGaugeValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func getCounterValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func getHistogramCount(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) uint64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

func gatherMetric(t *testing.T, registry *prometheus.Registry, name string) *promio.MetricFamily {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func findMetricByLabels(ms []*promio.Metric, labelPairs ...string) *promio.Metric {
	if len(labelPairs) == 0 {
		if len(ms) > 0 {
			return ms[0]
		}
		return nil
	}
	for _, m := range ms {
		if matchLabels(m.GetLabel(), labelPairs...) {
			return m
		}
	}
	return nil
}

func matchLabels(labels []*promio.LabelPair, pairs ...string) bool {
	for i := 0; i < len(pairs)-1; i += 2 {
		key, val := pairs[i], pairs[i+1]
		found := false
		for _, lp := range labels {
			if lp.GetName() == key && lp.GetValue() == val {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
