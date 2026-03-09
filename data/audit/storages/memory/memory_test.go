// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestStorage_Store(t *testing.T) {
	s := memory.New()
	ctx := t.Context()

	err := s.Store(ctx, &audit.Event{
		Type:   audit.EventTypeSystem,
		Action: audit.ActionExecute,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, s.Len())
}

func TestStorage_StoreBatch(t *testing.T) {
	s := memory.New()
	ctx := t.Context()

	events := []*audit.Event{
		{Type: audit.EventTypeSystem, Action: audit.ActionExecute},
		{Type: audit.EventTypeAuth, Action: audit.ActionLogin},
		{Type: audit.EventTypeDataChange, Action: audit.ActionCreate},
	}

	err := s.StoreBatch(ctx, events)
	require.NoError(t, err)
	assert.Equal(t, 3, s.Len())
}

func TestStorage_Query(t *testing.T) {
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
			name: "with_offset",
			events: []*audit.Event{
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
				{Actor: audit.Actor{ID: "u1"}, Timestamp: time.Now()},
			},
			query:   &audit.Query{ActorID: "u1", Offset: 1, Limit: 10},
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
				StartTime: testhelpers.TimePtr(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)),
				EndTime:   testhelpers.TimePtr(time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)),
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
			s := memory.New()
			ctx := t.Context()

			for _, e := range tt.events {
				require.NoError(t, s.Store(ctx, e))
			}

			var count int
			for range s.Query(ctx, tt.query) {
				count++
			}
			assert.Equal(t, tt.wantLen, count)
		})
	}
}

func TestStorage_Count(t *testing.T) {
	s := memory.New()
	ctx := t.Context()

	for i := range 5 {
		_ = s.Store(ctx, &audit.Event{
			Actor:     audit.Actor{ID: "u1"},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		})
	}
	_ = s.Store(ctx, &audit.Event{
		Actor:     audit.Actor{ID: "u2"},
		Timestamp: time.Now(),
	})

	count, err := s.Count(ctx, &audit.Query{ActorID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	count, err = s.Count(ctx, &audit.Query{})
	require.NoError(t, err)
	assert.Equal(t, int64(6), count)
}

func TestStorage_Events(t *testing.T) {
	s := memory.New()
	ctx := t.Context()

	_ = s.Store(ctx, &audit.Event{ID: "e1"})
	_ = s.Store(ctx, &audit.Event{ID: "e2"})

	events := s.Events()
	assert.Len(t, events, 2)
}

func TestStorage_Reset(t *testing.T) {
	s := memory.New()
	ctx := t.Context()

	_ = s.Store(ctx, &audit.Event{ID: "e1"})
	assert.Equal(t, 1, s.Len())

	s.Reset()
	assert.Equal(t, 0, s.Len())
}

func TestStorage_Close(t *testing.T) {
	s := memory.New()
	err := s.Close(t.Context())
	assert.NoError(t, err)
}
