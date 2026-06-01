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

// mustNew builds a Storage and fails the test on construction error.
func mustNew(tb testing.TB, maxHistoryPerTask int) *memory.Storage {
	tb.Helper()
	s, err := memory.New(maxHistoryPerTask)
	require.NoError(tb, err)
	return s
}

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
			s := mustNew(t, tt.input)
			assert.NotNil(t, s)
		})
	}
}

func TestStorage_GetTask_NotFound(t *testing.T) {
	s := mustNew(t, 100)
	ctx := t.Context()

	state, err := s.GetTask(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestStorage_UpsertAndGetTask(t *testing.T) {
	s := mustNew(t, 100)
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
	s := mustNew(t, 100)
	ctx := t.Context()

	state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "task-1", Status: scheduler.TaskStatusActive}}
	require.NoError(t, s.UpsertTask(ctx, state))

	state.Status = scheduler.TaskStatusPaused
	require.NoError(t, s.UpsertTask(ctx, state))

	got, _ := s.GetTask(ctx, "task-1")
	assert.Equal(t, scheduler.TaskStatusPaused, got.Status)
}

func TestStorage_DeleteTask(t *testing.T) {
	s := mustNew(t, 100)
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
	s := mustNew(t, 100)
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
	s := mustNew(t, 3) // limit 3
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
	s := mustNew(t, 100)
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
	s := mustNew(t, 100)
	ctx := t.Context()

	count := 0
	for range s.History(ctx, "nonexistent") {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestStorage_CleanupHistory(t *testing.T) {
	s := mustNew(t, 100)
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
	s := mustNew(t, 100)
	// Should not error
	err := s.DeleteTask(t.Context(), "nonexistent")
	assert.NoError(t, err)
}

func TestStorage_TasksPaginated(t *testing.T) {
	s := mustNew(t, 100)
	ctx := t.Context()

	// Insert 5 tasks
	for _, id := range []string{"e-task", "c-task", "a-task", "d-task", "b-task"} {
		_ = s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: id}})
	}

	// First page: AfterID="" means start from beginning
	page1, err := s.TasksPaginated(ctx, scheduler.Pagination{Limit: 2}, nil)
	require.NoError(t, err)
	// Should return 3 items (limit+1 for lookahead)
	assert.Len(t, page1, 3)
	assert.Equal(t, "a-task", page1[0].ID)
	assert.Equal(t, "b-task", page1[1].ID)
	assert.Equal(t, "c-task", page1[2].ID)

	// Second page: AfterID = "b-task" (last consumed from page 1)
	page2, err := s.TasksPaginated(ctx, scheduler.Pagination{AfterID: "b-task", Limit: 2}, nil)
	require.NoError(t, err)
	assert.Len(t, page2, 3)
	assert.Equal(t, "c-task", page2[0].ID)

	// Last page: AfterID = "d-task"
	page3, err := s.TasksPaginated(ctx, scheduler.Pagination{AfterID: "d-task", Limit: 2}, nil)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.Equal(t, "e-task", page3[0].ID)
}

func TestStorage_TasksPaginated_Empty(t *testing.T) {
	s := mustNew(t, 100)
	result, err := s.TasksPaginated(t.Context(), scheduler.Pagination{Limit: 10}, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestStorage_HistoryPaginated(t *testing.T) {
	s := mustNew(t, 100)
	ctx := t.Context()

	// Add 5 history entries with distinct StartedAt values
	for i := range 5 {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        "h-" + string(rune('a'+i)),
			TaskID:    "task-1",
			StartedAt: int64((i + 1) * 1000),
		})
	}

	// First page: most recent first, AfterID="" means from beginning
	page1, err := s.HistoryPaginated(ctx, "task-1", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 2}}, nil)
	require.NoError(t, err)
	// Should return 3 items (limit+1 for lookahead)
	assert.Len(t, page1, 3)
	// Descending order
	assert.Equal(t, int64(5000), page1[0].StartedAt)
	assert.Equal(t, int64(4000), page1[1].StartedAt)
	assert.Equal(t, int64(3000), page1[2].StartedAt)

	// Second page: after startedAt=4000, ID="h-d"
	page2, err := s.HistoryPaginated(ctx, "task-1", scheduler.HistoryPagination{
		Pagination:     scheduler.Pagination{AfterID: "h-d", Limit: 2},
		AfterStartedAt: 4000,
	}, nil)
	require.NoError(t, err)
	assert.Len(t, page2, 3)
	assert.Equal(t, int64(3000), page2[0].StartedAt)
}

func TestStorage_HistoryPaginated_Empty(t *testing.T) {
	s := mustNew(t, 100)
	result, err := s.HistoryPaginated(t.Context(), "nonexistent", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 10}}, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}
