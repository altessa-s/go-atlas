// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/metrics/adapters/prometheus"
	"github.com/altessa-s/go-atlas/observability/metrics"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	metricsfactory "github.com/altessa-s/go-atlas/factory/observability/metrics"
)

// inDefaultRegistry reports whether a metric family named name is exposed by
// the process-wide Prometheus registry.
func inDefaultRegistry(t *testing.T, name string) bool {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return true
		}
	}
	return false
}

func TestBuild_PrometheusRegistrySelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		service     string
		adapters    *observabilityconfig.MetricsAdapters
		wantDefault bool
	}{
		{
			name:        "custom registry keeps metrics out of the default one",
			service:     "covcustom",
			adapters:    &observabilityconfig.MetricsAdapters{Prometheus: &observabilityconfig.MetricsPrometheus{CustomRegistry: true}},
			wantDefault: false,
		},
		{
			name:        "default registry without adapter config",
			service:     "covdefault",
			wantDefault: true,
		},
		{
			name:        "default registry with empty adapter config",
			service:     "covempty",
			adapters:    &observabilityconfig.MetricsAdapters{},
			wantDefault: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := metricsfactory.New(&observabilityconfig.Metrics{
				Enabled:     true,
				Type:        observabilityconfig.MetricsTypePrometheus,
				ServiceName: tc.service,
				Adapters:    tc.adapters,
			}).UseAdapter(observabilityconfig.MetricsTypePrometheus, factory.Builder()).UseDefaultLogger().UseLogger(slog.New(slog.DiscardHandler)).Build()
			require.NoError(t, err)

			c.MustCounter(metrics.MetricOpts{Name: "hits_total", Help: "hits"}).Inc()
			require.Equal(t, tc.wantDefault, inDefaultRegistry(t, tc.service+"_hits_total"))
		})
	}
}

func TestCollectorBuilder_Build_Prometheus(t *testing.T) {
	cfg := &observabilityconfig.Metrics{
		Enabled:     true,
		Type:        observabilityconfig.MetricsTypePrometheus,
		ServiceName: "test",
	}

	collector, err := metricsfactory.New(cfg).UseAdapter(observabilityconfig.MetricsTypePrometheus, factory.Builder()).Build()
	require.NoError(t, err)
	require.NotNil(t, collector)
	require.False(t, metrics.IsNoop(collector), "expected real collector, not Noop")
}
