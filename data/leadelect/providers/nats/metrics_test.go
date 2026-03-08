// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestNatsLeaderMetrics_Noop(t *testing.T) {
	m := newNatsLeaderMetrics(nil)

	// Should not panic with noop metrics
	m.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.isLeader.Set(1)
	stop := m.campingIterations.Start()
	stop()
}

func TestNatsLeaderMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)
	m := newNatsLeaderMetrics(collector)

	m.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
	m.isLeader.Set(1)
	stop := m.campingIterations.Start()
	stop()

	val := getCounterValue(t, registry, "test_nats_leader_election_lease_operations_total",
		"op", "acquire", "result", "success")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_nats_leader_election_lease_operations_total",
		"op", "renew", "result", "failure")
	assert.Equal(t, float64(1), val)

	gauge := getGaugeValue(t, registry, "test_nats_leader_election_is_leader")
	assert.Equal(t, float64(1), gauge)

	count := getHistogramCount(t, registry, "test_nats_leader_election_camping_iteration_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))
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
