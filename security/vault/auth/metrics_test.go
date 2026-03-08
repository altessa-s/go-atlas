// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestVaultAuthMetrics_Noop(t *testing.T) {
	m := newVaultAuthMetrics(nil)

	// Should not panic with noop metrics
	m.authAttempts.Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "permanent"}).Inc()
	m.tokenRenewals.Inc()
	m.tokenRenewalErrors.Inc()
}

func TestVaultAuthMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)
	m := newVaultAuthMetrics(collector)

	m.authAttempts.Inc()
	m.authAttempts.Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "permanent"}).Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "transient"}).Inc()
	m.tokenRenewals.Inc()
	m.tokenRenewalErrors.Inc()

	val := getCounterValue(t, registry, "test_vault_auth_auth_attempts_total")
	assert.Equal(t, float64(2), val)

	val = getCounterValue(t, registry, "test_vault_auth_auth_errors_total", "type", "permanent")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_vault_auth_auth_errors_total", "type", "transient")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_vault_auth_token_renewals_total")
	assert.Equal(t, float64(1), val)

	val = getCounterValue(t, registry, "test_vault_auth_token_renewal_errors_total")
	assert.Equal(t, float64(1), val)
}

// --- helpers ---

func newTestCollector(registry *prometheus.Registry) metrics.Collector {
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	return metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))
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
