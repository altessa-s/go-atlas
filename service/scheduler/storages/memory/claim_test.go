// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

// activeTask builds an active task state due at nextRunAt.
func activeTask(t *testing.T, id string, nextRunAt int64) (*memory.Storage, *scheduler.TaskState) {
	t.Helper()
	s, err := memory.New(100)
	require.NoError(t, err)
	state := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: id, Status: scheduler.TaskStatusActive, NextRunAt: nextRunAt},
	}
	require.NoError(t, s.UpsertTask(t.Context(), state))
	return s, state
}

func TestClaimRun_SingleWinnerThenLost(t *testing.T) {
	t.Parallel()
	s, _ := activeTask(t, "a", 100)
	ctx := t.Context()

	ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.True(t, ok, "first claim must win")

	// The occupancy is now reflected in storage.
	got, err := s.GetTask(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, int64(1700000000), got.RunStartedAt)
	require.Equal(t, "run-1", got.LastRunID)

	// A second claim for the same occurrence loses (no longer active).
	ok, err = s.ClaimRun(ctx, "a", 100, 1700000001, "run-2")
	require.NoError(t, err)
	require.False(t, ok, "second claim must lose")
}

func TestClaimRun_FenceMismatch(t *testing.T) {
	t.Parallel()
	s, _ := activeTask(t, "a", 100)
	// Wrong occurrence fence: the task is active but scheduled for a different instant.
	ok, err := s.ClaimRun(t.Context(), "a", 999, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestClaimRun_ZeroFenceIgnoresNextRun(t *testing.T) {
	t.Parallel()
	s, _ := activeTask(t, "a", 100)
	// expectedNextRunAt==0 means "don't fence on next_run"; an active task is claimed.
	ok, err := s.ClaimRun(t.Context(), "a", 0, 1700000000, "run-1")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestClaimRun_NotActive(t *testing.T) {
	t.Parallel()
	s, state := activeTask(t, "a", 100)
	state.Status = scheduler.TaskStatusPaused
	require.NoError(t, s.UpsertTask(t.Context(), state))

	ok, err := s.ClaimRun(t.Context(), "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok, "paused task must not be claimable")
}

func TestClaimRun_MissingTask(t *testing.T) {
	t.Parallel()
	s, err := memory.New(100)
	require.NoError(t, err)
	ok, err := s.ClaimRun(t.Context(), "nope", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestClaimRun_ExactlyOneConcurrentWinner is the core anti-double-execution
// invariant: many schedulers racing on the same occurrence yield exactly one
// successful claim.
func TestClaimRun_ExactlyOneConcurrentWinner(t *testing.T) {
	t.Parallel()
	s, _ := activeTask(t, "a", 100)
	ctx := t.Context()

	const racers = 64
	var wins atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run")
			require.NoError(t, err)
			if ok {
				wins.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), wins.Load(), "exactly one claim may win")
}
