// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestLeaseMetrics_Noop(t *testing.T) {
	m := newLeaseMetrics(nil)

	// Should not panic with noop metrics
	m.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.leaseHeld.Set(1)
}

func TestLeaseMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)
	m := newLeaseMetrics(collector)

	m.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.operations.WithLabels(metrics.Labels{"op": "renew", "result": "success"}).Inc()
	m.operations.WithLabels(metrics.Labels{"op": "release", "result": "success"}).Inc()
	m.leaseHeld.Set(1)

	val := getCounterValue(t, registry, "test_nats_kv_lease_operations_total",
		"op", "acquire", "result", "success")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_nats_kv_lease_operations_total",
		"op", "renew", "result", "success")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_nats_kv_lease_operations_total",
		"op", "release", "result", "success")
	assert.Equal(t, float64(1), val)

	gauge := getGaugeValue(t, registry, "test_nats_kv_lease_lease_held")
	assert.Equal(t, float64(1), gauge)
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
