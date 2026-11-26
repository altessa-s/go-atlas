// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/dispatch"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/memory"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
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
	tc := metricstest.NewTestCollector()

	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng, audit.WithMeter(tc))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
	})

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))

	val := metricstest.GetCounterValue(t, tc, "test_audit_events_emitted_total")
	assert.Equal(t, float64(1), val, "events_emitted_total should be 1")
}

func TestAuditor_Metrics_EventsDropped(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()

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

	a, err := audit.New(eng, audit.WithMeter(tc))
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

	val := metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", audit.DropReasonQueueFull)
	assert.GreaterOrEqual(t, val, float64(1), "events_dropped_total should be >= 1")
}

func TestAuditor_Metrics_CustomSubsystem(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()

	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())

	a, err := audit.New(eng,
		audit.WithMeter(tc),
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

	val := metricstest.GetCounterValue(t, tc, "test_myaudit_events_emitted_total")
	assert.Equal(t, float64(1), val)
}

func TestAuditor_EmitLifecycleAndDropReasons(t *testing.T) {
	t.Parallel()

	t.Run("before_start_and_after_shutdown", func(t *testing.T) {
		t.Parallel()
		tc := metricstest.NewTestCollector()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

		store := memory.New()
		eng := newTestEngine(t, store)
		require.NoError(t, eng.Start())
		t.Cleanup(func() { _ = eng.Shutdown(context.WithoutCancel(t.Context())) })

		var hooks coreruntime.HookGroup
		a, err := audit.New(eng,
			audit.WithMeter(tc),
			audit.WithLogger(logger),
			audit.WithShutdownHooks(&hooks),
		)
		require.NoError(t, err)

		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionCreate}))
		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionCreate}))
		require.InDelta(t, 2.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", "not_started"), 0)
		require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "not-started drop warning must be logged once")
		require.NotContains(t, logs.String(), "buffer full")

		logs.Reset()
		require.NoError(t, a.Start())
		require.NoError(t, a.Shutdown(t.Context()))

		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
		require.InDelta(t, 2.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", "shutdown"), 0)
		require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "shutdown drop warning must be logged once")
		require.NotContains(t, logs.String(), "buffer full")
	})

	t.Run("closed_dispatcher_does_not_log_buffer_full", func(t *testing.T) {
		t.Parallel()
		tc := metricstest.NewTestCollector()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

		store := memory.New()
		eng := newTestEngine(t, store)
		require.NoError(t, eng.Start())

		var hooks coreruntime.HookGroup
		a, err := audit.New(eng,
			audit.WithMeter(tc),
			audit.WithLogger(logger),
			audit.WithShutdownHooks(&hooks),
		)
		require.NoError(t, err)
		require.NoError(t, a.Start())
		t.Cleanup(func() { _ = hooks.Shutdown(context.WithoutCancel(t.Context())) })

		require.NoError(t, eng.Shutdown(t.Context()))

		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute}))
		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute}))
		require.InDelta(t, 2.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", "dispatcher_closed"), 0)
		require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"))
		require.NotContains(t, logs.String(), "buffer full")
	})

	t.Run("submit_only_dispatcher_uses_generic_rejected_reason", func(t *testing.T) {
		t.Parallel()
		tc := metricstest.NewTestCollector()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

		a := startedAuditor(t, &captureDispatcher{full: true},
			audit.WithMeter(tc),
			audit.WithLogger(logger),
		)

		require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionRead}))
		require.InDelta(t, 1.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", "rejected"), 0)
		require.NotContains(t, logs.String(), "buffer full")
	})
}

func TestAuditor_RestartLifecycle(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())
	t.Cleanup(func() { _ = eng.Shutdown(context.WithoutCancel(t.Context())) })

	var hooks coreruntime.HookGroup
	a, err := audit.New(eng,
		audit.WithMeter(tc),
		audit.WithLogger(logger),
		audit.WithShutdownHooks(&hooks),
	)
	require.NoError(t, err)

	// First Start -> Emit -> Shutdown cycle.
	require.NoError(t, a.Start())
	require.True(t, a.Emit(&audit.Event{ID: "first-cycle", Type: audit.EventTypeBusinessEvent, Action: audit.ActionCreate}))

	require.NoError(t, a.Shutdown(t.Context()))
	require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
	require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
	require.InDelta(t, 2.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", audit.DropReasonShutdown), 0)
	require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "first shutdown must log drop warning once")

	// Restart over the still-running external dispatcher.
	logs.Reset()
	require.NoError(t, a.Start())
	require.ErrorIs(t, a.Start(), audit.ErrAuditorAlreadyStarted)
	require.True(t, a.Emit(&audit.Event{ID: "second-cycle", Type: audit.EventTypeBusinessEvent, Action: audit.ActionUpdate}))

	// Second Shutdown must stop admission again and log once for the new shutdown epoch.
	require.NoError(t, a.Shutdown(t.Context()))
	require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
	require.False(t, a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionDelete}))
	require.InDelta(t, 4.0, metricstest.GetCounterValue(t, tc, "test_audit_events_dropped_total", "reason", audit.DropReasonShutdown), 0)
	require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "second shutdown must log drop warning once")

	require.NoError(t, eng.Shutdown(t.Context()))
	require.Equal(t, 2, store.Len())
}
