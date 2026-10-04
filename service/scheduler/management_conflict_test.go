// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// conflictingStore performs a concurrent metadata write right before each of
// the first `conflicts` ReplaceTaskIf calls, so those calls lose the race.
type conflictingStore struct {
	*memory.Storage
	conflicts atomic.Int32
	attempts  atomic.Int32
}

func (s *conflictingStore) ReplaceTaskIf(ctx context.Context, state *scheduler.TaskState, expect scheduler.TaskFence) (bool, error) {
	s.attempts.Add(1)
	if s.conflicts.Add(-1) >= 0 {
		cur, err := s.Storage.GetTask(ctx, state.ID)
		if err != nil {
			return false, err
		}
		cur.Meta = map[string]string{"concurrent": "kept"}
		if err := s.Storage.UpsertTask(ctx, cur); err != nil {
			return false, err
		}
	}
	return s.Storage.ReplaceTaskIf(ctx, state, expect)
}

func newConflictScheduler(t *testing.T, conflicts int32) (*scheduler.Scheduler, *conflictingStore) {
	t.Helper()
	mem, err := memory.New(10)
	require.NoError(t, err)
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "task", Status: scheduler.TaskStatusActive, Schedule: "@every 1h", NextRunAt: 1 << 40,
	}}))
	store := &conflictingStore{Storage: mem}
	store.conflicts.Store(conflicts)
	return scheduler.New(store), store
}

func TestPauseTaskRetriesAndKeepsConcurrentWrite(t *testing.T) {
	t.Parallel()
	s, store := newConflictScheduler(t, 2)

	require.NoError(t, s.PauseTask(t.Context(), "task"))
	require.Equal(t, int32(3), store.attempts.Load())

	got, err := store.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusPaused, got.Status)
	require.Equal(t, "kept", got.Meta["concurrent"])
}

func TestManagementGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	s, store := newConflictScheduler(t, scheduler.MaxUpdateAttempts)

	require.ErrorIs(t, s.SkipNextRun(t.Context(), "task"), scheduler.ErrConcurrentUpdate)
	require.Equal(t, int32(scheduler.MaxUpdateAttempts), store.attempts.Load())

	got, err := store.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.False(t, got.SkipNextRun)
}

func TestRegisterKeepsInFlightRunFinishable(t *testing.T) {
	t.Parallel()
	mem, err := memory.New(10)
	require.NoError(t, err)
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary:  scheduler.TaskSummary{ID: "task", Status: scheduler.TaskStatusRunning, Schedule: "@every 1h", NextRunAt: 1 << 40},
		LastRunID:    "run",
		RunStartedAt: 5,
	}))
	s := scheduler.New(mem)

	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", Schedule: "@every 1h", Func: func(context.Context) error { return nil },
	}))

	ok, err := mem.FinishRun(t.Context(), "task", "run",
		scheduler.RunResult{StartedAt: 5, EndedAt: 6, NextRunAt: 1 << 40, Schedule: "@every 1h", Success: true})
	require.NoError(t, err)
	require.True(t, ok)
}
