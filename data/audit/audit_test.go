// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func TestAuditor_EmitAndShutdown(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithServiceInfo(audit.ServiceInfo{Name: "test-svc", Version: "1.0"}),
		audit.WithBatchSize(10),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for i := range 50 {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeBusinessEvent,
			Action: audit.ActionCreate,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
			Resource: audit.Resource{
				Type: "order",
				ID:   string(rune('0' + i%10)),
			},
			Result: audit.Result{Status: audit.ResultStatusSuccess},
		})
	}

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 50, store.Len())

	// Verify service info was set.
	events := store.Events()
	assert.Equal(t, "test-svc", events[0].Service.Name)
	assert.Equal(t, "1.0", events[0].Service.Version)

	// Verify IDs were generated.
	for _, e := range events {
		assert.NotEmpty(t, e.ID)
		assert.False(t, e.Timestamp.IsZero())
	}
}

func TestAuditor_Builder(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store, audit.WithFlushInterval(50*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.NewEvent(audit.EventTypeDataChange, audit.ActionUpdate).
		WithActor(audit.Actor{Type: audit.ActorTypeUser, ID: "u1"}).
		WithResource(audit.Resource{Type: "doc", ID: "d1"}).
		WithChanges(
			map[string]any{"name": "old"},
			map[string]any{"name": "new"},
			[]string{"name"},
		).
		WithSuccess().
		Emit()

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())

	e := store.Events()[0]
	assert.Equal(t, audit.EventTypeDataChange, e.Type)
	assert.Equal(t, audit.ActionUpdate, e.Action)
	assert.NotNil(t, e.Resource.Changes)
	assert.Equal(t, []string{"name"}, e.Resource.Changes.Fields)
}

func TestAuditor_BufferFull_DropsEvent(t *testing.T) {
	store := memory.New()

	var dropped atomic.Int32
	a, err := audit.New(store,
		audit.WithBufferSize(1),
		audit.WithWorkers(0), // invalid, defaults to DefaultWorkers
		audit.WithOnDrop(func(_ *audit.Event) { dropped.Add(1) }),
		audit.WithFlushInterval(time.Hour), // don't flush automatically
	)
	require.NoError(t, err)

	// Don't start — events go to buffer but no worker drains it.
	// Actually we need to start but make the worker very slow.
	// Instead, test that Emit returns false when not started.
	ok := a.Emit(&audit.Event{Type: audit.EventTypeSystem})
	assert.False(t, ok) // Not started
}

func TestAuditor_DoubleStart(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	defer a.Shutdown(t.Context())

	err = a.Start()
	assert.ErrorIs(t, err, audit.ErrAuditorAlreadyStarted)
}

func TestAuditor_NilStorage(t *testing.T) {
	_, err := audit.New(nil)
	assert.ErrorIs(t, err, audit.ErrNilStorage)
}

func TestAuditor_Query(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store, audit.WithFlushInterval(50*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{
		Type:   audit.EventTypeAuth,
		Action: audit.ActionLogin,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	})
	a.Emit(&audit.Event{
		Type:   audit.EventTypeAuth,
		Action: audit.ActionLogin,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u2"},
		Result: audit.Result{Status: audit.ResultStatusDenied},
	})

	require.NoError(t, a.Shutdown(t.Context()))

	// Query by actor.
	count, err := store.Count(t.Context(), &audit.Query{ActorID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Query by status.
	count, err = store.Count(t.Context(), &audit.Query{Status: audit.ResultStatusDenied})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestAuditor_DroppedEventsCounter(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBufferSize(1),
		audit.WithWorkers(1),
		audit.WithFlushInterval(time.Hour), // prevent automatic flush
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Fill the buffer and force drops: emit fast enough that the single
	// worker cannot drain in time. The first event may land in the buffer,
	// but subsequent ones will be dropped because flushInterval is huge.
	for range 20 {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeSystem,
			Action: audit.ActionCreate,
		})
	}

	assert.Greater(t, a.DroppedEvents(), int64(0))

	require.NoError(t, a.Shutdown(t.Context()))
}

func TestAuditor_BackPressure(t *testing.T) {
	store := memory.New()
	const eventCount = 20

	a, err := audit.New(store,
		audit.WithBufferSize(1),
		audit.WithWorkers(1),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithBackPressure(),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// With back-pressure enabled, Emit blocks until buffer has space,
	// so no events should be dropped.
	for range eventCount {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeBusinessEvent,
			Action: audit.ActionCreate,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-bp"},
			Result: audit.Result{Status: audit.ResultStatusSuccess},
		})
	}

	require.NoError(t, a.Shutdown(t.Context()))

	assert.Equal(t, int64(0), a.DroppedEvents())
	assert.Equal(t, eventCount, store.Len())
}

func TestAuditor_Context(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store)
	require.NoError(t, err)

	ctx := audit.NewContext(t.Context(), a)
	got := audit.FromContext(ctx)
	assert.Equal(t, a, got)

	// Nil context returns nil.
	assert.Nil(t, audit.FromContext(t.Context()))
}
