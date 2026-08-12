// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

// TestDueTasks pins the predicate the scheduler's tick depends on. The filter
// lives in the storage layer precisely so the tick does not pay for tasks that
// are not due, which means a wrong predicate here is invisible from the tick's
// side: too narrow and tasks silently stop firing, too wide and they fire
// early — the one axis ClaimRun's compare-and-swap does not backstop.
func TestDueTasks(t *testing.T) {
	t.Parallel()

	const now int64 = 1000

	seed := []*scheduler.TaskState{
		{TaskSummary: scheduler.TaskSummary{ID: "b-due", Status: scheduler.TaskStatusActive, NextRunAt: now - 1}},
		{TaskSummary: scheduler.TaskSummary{ID: "a-due", Status: scheduler.TaskStatusActive, NextRunAt: now - 100}},
		{TaskSummary: scheduler.TaskSummary{ID: "c-exactly-now", Status: scheduler.TaskStatusActive, NextRunAt: now}},
		{TaskSummary: scheduler.TaskSummary{ID: "d-future", Status: scheduler.TaskStatusActive, NextRunAt: now + 1}},
		{TaskSummary: scheduler.TaskSummary{ID: "e-paused", Status: scheduler.TaskStatusPaused, NextRunAt: now - 1}},
		{TaskSummary: scheduler.TaskSummary{ID: "f-disabled", Status: scheduler.TaskStatusDisabled, NextRunAt: now - 1}},
		{TaskSummary: scheduler.TaskSummary{ID: "g-completed", Status: scheduler.TaskStatusCompleted, NextRunAt: now - 1}},
		{TaskSummary: scheduler.TaskSummary{ID: "h-running", Status: scheduler.TaskStatusRunning, NextRunAt: now - 1}},
	}

	storage, err := memory.New(10)
	require.NoError(t, err)

	ctx := t.Context()
	for _, st := range seed {
		require.NoError(t, storage.UpsertTask(ctx, st))
	}

	var got []string
	for state, err := range storage.DueTasks(ctx, now) {
		require.NoError(t, err)
		got = append(got, state.ID)
	}

	// Active and at-or-before now, ordered by ID. The boundary is inclusive:
	// NextRunAt is the instant the occurrence becomes eligible, not the
	// instant after it.
	require.Equal(t, []string{"a-due", "b-due", "c-exactly-now"}, got)
}

// TestDueTasks_ReturnsCopies guards the same isolation contract the other
// readers honor: a caller mutating a yielded state must not corrupt the store.
func TestDueTasks_ReturnsCopies(t *testing.T) {
	t.Parallel()

	storage, err := memory.New(10)
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, storage.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:        "task",
			Status:    scheduler.TaskStatusActive,
			NextRunAt: 1,
		},
	}))

	for state, err := range storage.DueTasks(ctx, 10) {
		require.NoError(t, err)
		state.Status = scheduler.TaskStatusDisabled
		state.NextRunAt = 9999
	}

	stored, err := storage.GetTask(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusActive, stored.Status)
	require.Equal(t, int64(1), stored.NextRunAt)
}
