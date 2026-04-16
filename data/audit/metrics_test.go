// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// blockingSink implements [dispatch.Sink] and blocks until the channel is closed.
type blockingSink struct {
	blocked chan struct{}
}

func (s *blockingSink) StoreBatch(_ context.Context, _ []*audit.Event) error {
	<-s.blocked
	return nil
}

func TestAuditor_Metrics_Noop(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))
	// Should not panic with noop metrics.
}

func TestAuditor_Metrics_EventsEmitted(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng, audit.WithCollector(collector))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))

	val := testhelpers.GetCounterValue(t, registry, "test_audit_events_emitted_total")
	assert.Equal(t, float64(1), val, "events_emitted_total should be 1")
}

func TestAuditor_Metrics_EventsDropped(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

	// Use a blocking sink that never returns, so the single worker stays
	// busy and the buffer (size 1) fills up, causing drops.
	blocked := make(chan struct{})

	sink := &blockingSink{blocked: blocked}

	eng, err := dispatch.NewEngine[*audit.Event](sink,
		dispatch.WithBufferSize[*audit.Event](1),
		dispatch.WithFlushInterval[*audit.Event](time.Millisecond),
		dispatch.WithBatchSize[*audit.Event](1),
		dispatch.WithWorkers[*audit.Event](1),
	)
	require.NoError(t, err)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng, audit.WithCollector(collector))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Wait briefly so the worker picks up the first item and blocks on sink.
	time.Sleep(20 * time.Millisecond) //nolint:mnd // give worker time to block

	// Now the channel buffer (1) might have room for one more, but the rest
	// will be dropped.
	for range 20 {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeBusinessEvent,
			Action: audit.ActionCreate,
		})
	}

	require.NoError(t, a.Shutdown(t.Context()))
	// Unblock the sink so the engine worker can drain and Shutdown returns.
	close(blocked)
	require.NoError(t, eng.Shutdown(t.Context()))

	val := testhelpers.GetCounterValue(t, registry, "test_audit_events_dropped_total")
	assert.GreaterOrEqual(t, val, float64(1), "events_dropped_total should be >= 1")
}

func TestAuditor_Metrics_CustomSubsystem(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)

	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng,
		audit.WithCollector(collector),
		audit.WithMetricsSubsystem("myaudit"),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))

	val := testhelpers.GetCounterValue(t, registry, "test_myaudit_events_emitted_total")
	assert.Equal(t, float64(1), val)
}
