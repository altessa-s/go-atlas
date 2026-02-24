// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

func TestNew_DefaultMaxHistory(t *testing.T) {
	tests := []struct {
		name  string
		input int
	}{
		{"zero_defaults_to_1000", 0},
		{"negative_defaults_to_1000", -1},
		{"positive_uses_value", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := memory.New(tt.input)
			assert.NotNil(t, s)
		})
	}
}

func TestStorage_GetTask_NotFound(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	state, err := s.GetTask(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestStorage_UpsertAndGetTask(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	state := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:       "task-1",
			Status:   scheduler.TaskStatusActive,
			Schedule: "@every 1h",
		},
		Meta: map[string]string{"env": "test"},
	}

	require.NoError(t, s.UpsertTask(ctx, state))

	got, err := s.GetTask(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "task-1", got.ID)
	assert.Equal(t, scheduler.TaskStatusActive, got.Status)
	assert.Equal(t, "test", got.Meta["env"])

	// Verify it's a clone (mutation doesn't affect stored)
	got.Status = scheduler.TaskStatusPaused
	got2, _ := s.GetTask(ctx, "task-1")
	assert.Equal(t, scheduler.TaskStatusActive, got2.Status)
}

func TestStorage_UpsertTask_Update(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "task-1", Status: scheduler.TaskStatusActive}}
	require.NoError(t, s.UpsertTask(ctx, state))

	state.Status = scheduler.TaskStatusPaused
	require.NoError(t, s.UpsertTask(ctx, state))

	got, _ := s.GetTask(ctx, "task-1")
	assert.Equal(t, scheduler.TaskStatusPaused, got.Status)
}

func TestStorage_DeleteTask(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	_ = s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "task-1", Status: scheduler.TaskStatusActive}})
	_ = s.AddHistory(ctx, &scheduler.TaskHistory{ID: "h1", TaskID: "task-1"})

	require.NoError(t, s.DeleteTask(ctx, "task-1"))

	got, err := s.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Nil(t, got)

	// History should also be deleted
	count := 0
	for range s.History(ctx, "task-1") {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestStorage_Tasks_SortedByID(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	_ = s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "c-task"}})
	_ = s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "a-task"}})
	_ = s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "b-task"}})

	var ids []string
	for state, err := range s.Tasks(ctx) {
		require.NoError(t, err)
		ids = append(ids, state.ID)
	}

	assert.Equal(t, []string{"a-task", "b-task", "c-task"}, ids)
}

func TestStorage_AddHistory_TrimOverLimit(t *testing.T) {
	s := memory.New(3) // limit 3
	ctx := t.Context()

	for i := range 5 {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        string(rune('a' + i)),
			TaskID:    "task-1",
			StartedAt: int64(i),
		})
	}

	var count int
	for range s.History(ctx, "task-1") {
		count++
	}
	assert.Equal(t, 3, count)
}

func TestStorage_History_SortedDescending(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	for i := range 5 {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        string(rune('a' + i)),
			TaskID:    "task-1",
			StartedAt: int64(i * 100),
		})
	}

	var prev int64 = int64(^uint64(0) >> 1) // max int64
	for h, err := range s.History(ctx, "task-1") {
		require.NoError(t, err)
		assert.LessOrEqual(t, h.StartedAt, prev)
		prev = h.StartedAt
	}
}

func TestStorage_History_Empty(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	count := 0
	for range s.History(ctx, "nonexistent") {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestStorage_CleanupHistory(t *testing.T) {
	s := memory.New(100)
	ctx := t.Context()

	now := time.Now()
	_ = s.AddHistory(ctx, &scheduler.TaskHistory{
		ID:      "old",
		TaskID:  "task-1",
		EndedAt: now.Add(-48 * time.Hour).Unix(),
	})
	_ = s.AddHistory(ctx, &scheduler.TaskHistory{
		ID:      "new",
		TaskID:  "task-1",
		EndedAt: now.Unix(),
	})

	require.NoError(t, s.CleanupHistory(ctx, 24*time.Hour))

	count := 0
	for range s.History(ctx, "task-1") {
		count++
	}
	assert.Equal(t, 1, count)
}

func TestStorage_DeleteTask_Nonexistent(t *testing.T) {
	s := memory.New(100)
	// Should not error
	err := s.DeleteTask(t.Context(), "nonexistent")
	assert.NoError(t, err)
}
