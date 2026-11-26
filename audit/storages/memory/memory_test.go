// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/memory"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

func TestStorage_Store(t *testing.T) {
	t.Parallel()

	s := memory.New()
	ctx := t.Context()

	err := s.Store(ctx, &audit.Event{
		Type:   audit.EventTypeSystem,
		Action: audit.ActionExecute,
	})
	require.NoError(t, err)
	require.Equal(t, 1, s.Len())
}

func TestStorage_StoreBatch(t *testing.T) {
	t.Parallel()

	s := memory.New()
	ctx := t.Context()

	events := []*audit.Event{
		{Type: audit.EventTypeSystem, Action: audit.ActionExecute},
		{Type: audit.EventTypeAuth, Action: audit.ActionLogin},
		{Type: audit.EventTypeDataChange, Action: audit.ActionCreate},
	}

	err := s.StoreBatch(ctx, events)
	require.NoError(t, err)
	require.Equal(t, 3, s.Len())
}

func TestStorage_Query(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		events  []*audit.Event
		query   *audit.Query
		wantLen int
	}{
		{
			name: "by_actor_id",
			events: []*audit.Event{
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
				{Actor: audit.Actor{ID: "u2"}, Timestamp: time.Now()},
			},
			query:   &audit.Query{ActorID: "u1"},
			wantLen: 1,
		},
		{
			name: "by_resource_type",
			events: []*audit.Event{
				{Resource: audit.Resource{Type: "order"}, Timestamp: time.Now()},
				{Resource: audit.Resource{Type: "user"}, Timestamp: time.Now()},
				{Resource: audit.Resource{Type: "order"}, Timestamp: time.Now()},
			},
			query:   &audit.Query{ResourceType: "order"},
			wantLen: 2,
		},
		{
			name: "by_event_type",
			events: []*audit.Event{
				{Type: audit.EventTypeAuth, Timestamp: time.Now()},
				{Type: audit.EventTypeSystem, Timestamp: time.Now()},
			},
			query:   &audit.Query{EventType: audit.EventTypeAuth},
			wantLen: 1,
		},
		{
			name: "by_action",
			events: []*audit.Event{
				{Action: audit.ActionCreate, Timestamp: time.Now()},
				{Action: audit.ActionDelete, Timestamp: time.Now()},
			},
			query:   &audit.Query{Action: audit.ActionCreate},
			wantLen: 1,
		},
		{
			name: "by_status",
			events: []*audit.Event{
				{Result: audit.Result{Status: audit.ResultStatusSuccess}, Timestamp: time.Now()},
				{Result: audit.Result{Status: audit.ResultStatusFailure}, Timestamp: time.Now()},
			},
			query:   &audit.Query{Status: audit.ResultStatusSuccess},
			wantLen: 1,
		},
		{
			name: "with_limit",
			events: []*audit.Event{
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
			},
			query:   &audit.Query{ActorID: "u1", Limit: 2},
			wantLen: 2,
		},
		{
			name: "with_cursor",
			events: []*audit.Event{
				{ID: "a", Actor: audit.Actor{ID: "u1"}, Timestamp: time.UnixMilli(1000)},
				{ID: "b", Actor: audit.Actor{ID: "u1"}, Timestamp: time.UnixMilli(1000)},
				{ID: "c", Actor: audit.Actor{ID: "u1"}, Timestamp: time.UnixMilli(1000)},
			},
			query:   &audit.Query{ActorID: "u1", Limit: 10, Cursor: &audit.Cursor{Timestamp: time.UnixMilli(1000), ID: "c"}},
			wantLen: 2,
		},
		{
			name: "by_time_range",
			events: []*audit.Event{
				{Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				{Timestamp: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
				{Timestamp: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)},
			},
			query: &audit.Query{
				StartTime: new(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)),
				EndTime:   new(time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)),
			},
			wantLen: 1,
		},
		{
			name: "by_trace_id",
			events: []*audit.Event{
				{Context: audit.EventContext{TraceID: "t1"}, Timestamp: time.Now()},
				{Context: audit.EventContext{TraceID: "t2"}, Timestamp: time.Now()},
			},
			query:   &audit.Query{TraceID: "t1"},
			wantLen: 1,
		},
		{
			name:    "empty_storage",
			events:  nil,
			query:   &audit.Query{ActorID: "u1"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := memory.New()
			ctx := t.Context()

			for _, e := range tt.events {
				require.NoError(t, s.Store(ctx, e))
			}

			var count int
			for range s.Query(ctx, tt.query) {
				count++
			}
			require.Equal(t, tt.wantLen, count)
		})
	}
}

func TestStorage_Count(t *testing.T) {
	t.Parallel()

	s := memory.New()
	ctx := t.Context()

	for i := range 5 {
		require.NoError(t, s.Store(ctx, &audit.Event{
			Actor:     audit.Actor{ID: "u1"},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}))
	}
	require.NoError(t, s.Store(ctx, &audit.Event{
		Actor:     audit.Actor{ID: "u2"},
		Timestamp: time.Now(),
	}))

	count, err := s.Count(ctx, &audit.Query{ActorID: "u1"})
	require.NoError(t, err)
	require.Equal(t, int64(5), count)

	count, err = s.Count(ctx, &audit.Query{})
	require.NoError(t, err)
	require.Equal(t, int64(6), count)
}

func TestStorage_Events(t *testing.T) {
	t.Parallel()

	s := memory.New()
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, &audit.Event{ID: "e1"}))
	require.NoError(t, s.Store(ctx, &audit.Event{ID: "e2"}))

	events := s.Events()
	require.Len(t, events, 2)
}

func TestStorage_Reset(t *testing.T) {
	t.Parallel()

	s := memory.New()
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, &audit.Event{ID: "e1"}))
	require.Equal(t, 1, s.Len())

	s.Reset()
	require.Equal(t, 0, s.Len())
}

func TestStorage_Close(t *testing.T) {
	t.Parallel()

	s := memory.New()
	require.NoError(t, s.Close(t.Context()))
}

func TestStorage_DefaultBoundedCapacity(t *testing.T) {
	t.Parallel()

	s := memory.New()
	require.Equal(t, memory.DefaultMaxEvents, s.Cap())

	const overflow = 5
	batch := make([]*audit.Event, memory.DefaultMaxEvents+overflow)
	for i := range batch {
		batch[i] = &audit.Event{
			ID:        fmt.Sprintf("ev-%d", i),
			Timestamp: time.UnixMilli(int64(i)),
		}
	}
	require.NoError(t, s.StoreBatch(t.Context(), batch))
	require.Equal(t, memory.DefaultMaxEvents, s.Len())

	got := s.Events()
	require.Len(t, got, memory.DefaultMaxEvents)
	require.Equal(t, "ev-5", got[0].ID)
	require.Equal(t, fmt.Sprintf("ev-%d", memory.DefaultMaxEvents+overflow-1), got[len(got)-1].ID)
}

func TestStorage_RingBufferEvictsOldest(t *testing.T) {
	t.Parallel()

	// Non-positive capacities are ignored in favor of DefaultMaxEvents.
	require.Equal(t, memory.DefaultMaxEvents, memory.New(memory.WithMaxEvents(0)).Cap())
	require.Equal(t, memory.DefaultMaxEvents, memory.New(memory.WithMaxEvents(-3)).Cap())

	s := memory.New(memory.WithMaxEvents(3))
	require.Equal(t, 3, s.Cap())
	ctx := t.Context()

	for i := 1; i <= 5; i++ {
		require.NoError(t, s.Store(ctx, &audit.Event{
			ID:        fmt.Sprintf("e%d", i),
			Timestamp: time.UnixMilli(int64(i)),
		}))
	}
	require.Equal(t, 3, s.Len())
	require.Equal(t, []string{"e3", "e4", "e5"}, eventIDs(s.Events()))

	// Partial wrap via StoreBatch.
	require.NoError(t, s.StoreBatch(ctx, []*audit.Event{
		{ID: "e6", Timestamp: time.UnixMilli(6)},
		{ID: "e7", Timestamp: time.UnixMilli(7)},
	}))
	require.Equal(t, []string{"e5", "e6", "e7"}, eventIDs(s.Events()))

	// Nil element in StoreBatch rejects atomically without evicting existing items.
	err := s.StoreBatch(ctx, []*audit.Event{
		{ID: "e8", Timestamp: time.UnixMilli(8)},
		nil,
	})
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Equal(t, []string{"e5", "e6", "e7"}, eventIDs(s.Events()))

	// Batch larger than ring capacity retains only the newest Cap() events.
	require.NoError(t, s.StoreBatch(ctx, []*audit.Event{
		{ID: "e8", Timestamp: time.UnixMilli(8)},
		{ID: "e9", Timestamp: time.UnixMilli(9)},
		{ID: "e10", Timestamp: time.UnixMilli(10)},
		{ID: "e11", Timestamp: time.UnixMilli(11)},
	}))
	require.Equal(t, []string{"e9", "e10", "e11"}, eventIDs(s.Events()))

	count, err := s.Count(ctx, nil)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

	var queried []string
	for ev, qErr := range s.Query(ctx, &audit.Query{SortOrder: audit.SortOrderAsc}) {
		require.NoError(t, qErr)
		queried = append(queried, ev.ID)
	}
	require.Equal(t, []string{"e9", "e10", "e11"}, queried)
}

func TestStorage_ReleasesEvictedAndResetEvents(t *testing.T) {
	s := memory.New(memory.WithMaxEvents(1))
	ctx := t.Context()

	var evictedRef, resetRef weak.Pointer[audit.Event]
	func() {
		evicted := &audit.Event{ID: "first"}
		evictedRef = weak.Make(evicted)
		require.NoError(t, s.Store(ctx, evicted))

		kept := &audit.Event{ID: "second"}
		resetRef = weak.Make(kept)
		require.NoError(t, s.Store(ctx, kept))
	}()

	runtime.GC()
	require.Nil(t, evictedRef.Value(), "evicted event must not remain reachable from the ring buffer")

	s.Reset()
	runtime.GC()
	require.Nil(t, resetRef.Value(), "reset must clear backing references")
}

func eventIDs(events []*audit.Event) []string {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}
