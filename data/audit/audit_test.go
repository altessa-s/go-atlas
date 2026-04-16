// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// newTestEngine creates a dispatch.Engine backed by the given store with
// sensible test defaults. The caller can override defaults via extra opts.
func newTestEngine(
	tb testing.TB,
	store audit.Storage,
	extra ...dispatch.Option[*audit.Event],
) *dispatch.Engine[*audit.Event] {
	tb.Helper()
	base := []dispatch.Option[*audit.Event]{
		dispatch.WithBatchSize[*audit.Event](10),
		dispatch.WithFlushInterval[*audit.Event](20 * time.Millisecond),
		dispatch.WithWorkers[*audit.Event](2),
	}
	eng, err := dispatch.NewEngine[*audit.Event](
		audit.StorageSink{Storage: store},
		append(base, extra...)...,
	)
	require.NoError(tb, err)
	return eng
}

func TestAuditor_EmitAndShutdown(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store,
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond),
		dispatch.WithWorkers[*audit.Event](1),
	)
	require.NoError(t, eng.Start())
	a, err := audit.New(eng,
		audit.WithServiceInfo(audit.ServiceInfo{Name: "test-svc", Version: "1.0"}),
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
	require.NoError(t, eng.Shutdown(t.Context()))
	assert.Equal(t, 50, store.Len())

	events := store.Events()
	assert.Equal(t, "test-svc", events[0].Service.Name)
	assert.Equal(t, "1.0", events[0].Service.Version)

	for _, e := range events {
		assert.NotEmpty(t, e.ID)
		assert.False(t, e.Timestamp.IsZero())
	}
}

func TestAuditor_Builder(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store,
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond),
	)
	require.NoError(t, eng.Start())
	a, err := audit.New(eng)
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
	require.NoError(t, eng.Shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())

	e := store.Events()[0]
	assert.Equal(t, audit.EventTypeDataChange, e.Type)
	assert.Equal(t, audit.ActionUpdate, e.Action)
	assert.NotNil(t, e.Resource.Changes)
	assert.Equal(t, []string{"name"}, e.Resource.Changes.Fields)
}

func TestAuditor_NotStarted_EmitReturnsFalse(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())
	defer eng.Shutdown(t.Context()) //nolint:errcheck // test cleanup
	a, err := audit.New(eng)
	require.NoError(t, err)

	ok := a.Emit(&audit.Event{Type: audit.EventTypeSystem})
	assert.False(t, ok)
}

func TestAuditor_DoubleStart(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())
	defer eng.Shutdown(t.Context()) //nolint:errcheck // test cleanup
	a, err := audit.New(eng)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	defer a.Shutdown(t.Context())

	err = a.Start()
	assert.ErrorIs(t, err, audit.ErrAuditorAlreadyStarted)
}

func TestAuditor_NilEngine(t *testing.T) {
	t.Parallel()
	_, err := audit.New(nil)
	assert.ErrorIs(t, err, audit.ErrNilDispatcher)
}

func TestAuditor_Query(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store,
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond),
	)
	require.NoError(t, eng.Start())
	a, err := audit.New(eng)
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
	require.NoError(t, eng.Shutdown(t.Context()))

	count, err := store.Count(t.Context(), &audit.Query{ActorID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	count, err = store.Count(t.Context(), &audit.Query{Status: audit.ResultStatusDenied})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestAuditor_DroppedEventsCounter(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store,
		dispatch.WithBufferSize[*audit.Event](1),
		dispatch.WithWorkers[*audit.Event](1),
		dispatch.WithFlushInterval[*audit.Event](time.Hour),
	)
	require.NoError(t, eng.Start())
	a, err := audit.New(eng)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for range 20 {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeSystem,
			Action: audit.ActionCreate,
		})
	}

	assert.Greater(t, a.DroppedEvents(), int64(0))

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))
}

func TestAuditor_BackPressure(t *testing.T) {
	t.Parallel()
	store := memory.New()
	const eventCount = 20

	eng := newTestEngine(t, store,
		dispatch.WithBufferSize[*audit.Event](1),
		dispatch.WithWorkers[*audit.Event](1),
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond),
		dispatch.WithBackPressure[*audit.Event](),
	)
	require.NoError(t, eng.Start())
	a, err := audit.New(eng)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for range eventCount {
		a.Emit(&audit.Event{
			Type:   audit.EventTypeBusinessEvent,
			Action: audit.ActionCreate,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-bp"},
			Result: audit.Result{Status: audit.ResultStatusSuccess},
		})
	}

	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, eng.Shutdown(t.Context()))

	assert.Equal(t, int64(0), a.DroppedEvents())
	assert.Equal(t, eventCount, store.Len())
}

func TestAuditor_Context(t *testing.T) {
	t.Parallel()
	store := memory.New()
	eng := newTestEngine(t, store)
	require.NoError(t, eng.Start())
	defer eng.Shutdown(t.Context()) //nolint:errcheck // test cleanup
	a, err := audit.New(eng)
	require.NoError(t, err)

	ctx := audit.NewContext(t.Context(), a)
	got := audit.FromContext(ctx)
	assert.Equal(t, a, got)

	assert.Nil(t, audit.FromContext(t.Context()))
}
