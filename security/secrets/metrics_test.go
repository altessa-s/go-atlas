// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestSecretsMetrics_Noop(t *testing.T) {
	m := newSecretsMetrics(nil)

	// Should not panic with noop metrics
	m.cacheHits.Inc()
	m.cacheMisses.Inc()
	stop := m.fetchDuration.Start()
	stop()
	stop = m.updateCycleDuration.Start()
	stop()
	m.updateCycleErrors.Inc()
	m.cacheSize.Set(10)
}

func TestSecretsMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)
	m := newSecretsMetrics(collector)

	m.cacheHits.Inc()
	m.cacheHits.Inc()
	m.cacheMisses.Inc()
	stop := m.fetchDuration.Start()
	stop()
	stop = m.updateCycleDuration.Start()
	stop()
	m.updateCycleErrors.Inc()
	m.cacheSize.Set(42)

	val := getCounterValue(t, registry, "test_secrets_cache_hits_total")
	assert.Equal(t, float64(2), val)

	val = getCounterValue(t, registry, "test_secrets_cache_misses_total")
	assert.Equal(t, float64(1), val)

	count := getHistogramCount(t, registry, "test_secrets_fetch_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	count = getHistogramCount(t, registry, "test_secrets_update_cycle_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	val = getCounterValue(t, registry, "test_secrets_update_cycle_errors_total")
	assert.Equal(t, float64(1), val)

	gauge := getGaugeValue(t, registry, "test_secrets_cache_size")
	assert.Equal(t, float64(42), gauge)
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
