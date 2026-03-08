// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/observability/metrics"

	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestAuditor_Metrics_Noop(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))
	// Should not panic with noop metrics
}

func TestAuditor_Metrics_EventsEmitted(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	store := memory.New()
	a, err := audit.New(store,
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
		audit.WithCollector(collector),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))

	val := getCounterValue(t, registry, "test_audit_events_emitted_total")
	assert.Equal(t, float64(1), val, "events_emitted_total should be 1")
}

func TestAuditor_Metrics_EventsDropped(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	store := memory.New()
	a, err := audit.New(store,
		audit.WithBufferSize(1),
		audit.WithFlushInterval(time.Hour), // long interval to force buffer fill
		audit.WithWorkers(0),               // no workers to prevent draining
		audit.WithCollector(collector),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Fill the buffer and then drop
	for range 10 {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeBusinessEvent,
			Action: audit.ActionCreate,
		})
	}

	require.NoError(t, a.Shutdown(t.Context()))

	val := getCounterValue(t, registry, "test_audit_events_dropped_total")
	assert.GreaterOrEqual(t, val, float64(1), "events_dropped_total should be >= 1")
}

func TestAuditor_Metrics_WorkersActive(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	store := memory.New()
	a, err := audit.New(store,
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(2),
		audit.WithCollector(collector),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Give workers time to start
	time.Sleep(50 * time.Millisecond)

	val := getGaugeValue(t, registry, "test_audit_workers_active")
	assert.Equal(t, float64(2), val, "workers_active should be 2")

	require.NoError(t, a.Shutdown(t.Context()))

	val = getGaugeValue(t, registry, "test_audit_workers_active")
	assert.Equal(t, float64(0), val, "workers_active should be 0 after shutdown")
}

func TestAuditor_Metrics_FlushDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := newTestCollector(registry)

	store := memory.New()
	a, err := audit.New(store,
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithBatchSize(1),
		audit.WithWorkers(1),
		audit.WithCollector(collector),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))

	count := getHistogramCount(t, registry, "test_audit_batch_flush_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1), "batch_flush_duration_seconds should have >= 1 observation")
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
