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
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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
	collector := testhelpers.NewTestCollector(registry)

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

	val := testhelpers.GetCounterValue(t, registry, "test_audit_events_emitted_total")
	assert.Equal(t, float64(1), val, "events_emitted_total should be 1")
}

func TestAuditor_Metrics_EventsDropped(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

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

	val := testhelpers.GetCounterValue(t, registry, "test_audit_events_dropped_total")
	assert.GreaterOrEqual(t, val, float64(1), "events_dropped_total should be >= 1")
}

func TestAuditor_Metrics_WorkersActive(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

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

	val := testhelpers.GetGaugeValue(t, registry, "test_audit_workers_active")
	assert.Equal(t, float64(2), val, "workers_active should be 2")

	require.NoError(t, a.Shutdown(t.Context()))

	val = testhelpers.GetGaugeValue(t, registry, "test_audit_workers_active")
	assert.Equal(t, float64(0), val, "workers_active should be 0 after shutdown")
}

func TestAuditor_Metrics_FlushDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

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

	count := testhelpers.GetHistogramCount(t, registry, "test_audit_batch_flush_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1), "batch_flush_duration_seconds should have >= 1 observation")
}
