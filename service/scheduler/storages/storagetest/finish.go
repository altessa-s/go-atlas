// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// FinishRun verifies ownership, idempotent completion and preservation of
// configuration changes. The supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps and counts describe the storage contract.
func FinishRun(t *testing.T, store scheduler.Storage) {
	t.Helper()
	result := scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 200, Schedule: "@every 1m", Success: true}
	for _, tc := range []struct {
		name       string
		status     scheduler.TaskStatus
		oneShot    bool
		schedule   string
		wantStatus scheduler.TaskStatus
		wantNext   int64
	}{
		{"normal", scheduler.TaskStatusRunning, false, "@every 1m", scheduler.TaskStatusActive, 200},
		{"paused", scheduler.TaskStatusPaused, false, "@every 1m", scheduler.TaskStatusPaused, 200},
		{"disabled", scheduler.TaskStatusDisabled, false, "@every 1m", scheduler.TaskStatusDisabled, 200},
		{"changed_schedule", scheduler.TaskStatusRunning, false, "@every 2m", scheduler.TaskStatusActive, 300},
		{"one_shot", scheduler.TaskStatusRunning, true, "", scheduler.TaskStatusCompleted, 0},
		// A one-shot run that finished is terminal whatever management did
		// meanwhile: paused, disabled, or paused and resumed back to active.
		{"one_shot_paused", scheduler.TaskStatusPaused, true, "", scheduler.TaskStatusCompleted, 0},
		{"one_shot_disabled", scheduler.TaskStatusDisabled, true, "", scheduler.TaskStatusCompleted, 0},
		{"one_shot_resumed", scheduler.TaskStatusActive, true, "", scheduler.TaskStatusCompleted, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
				ID: tc.name, Status: tc.status, Schedule: tc.schedule, NextRunAt: 300, Failures: 2, OneShot: tc.oneShot,
			}, LastRunID: "owner", RunStartedAt: 100, RunLeaseUntil: 400, RunLeaseID: "owner", Meta: map[string]string{"keep": "value"}}
			require.NoError(t, store.UpsertTask(t.Context(), state))
			ok, err := store.FinishRun(t.Context(), tc.name, "stale", result)
			require.NoError(t, err)
			require.False(t, ok)
			ok, err = store.FinishRun(t.Context(), tc.name, "owner", result)
			require.NoError(t, err)
			require.True(t, ok)
			got, err := store.GetTask(t.Context(), tc.name)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, tc.wantNext, got.NextRunAt)
			require.Equal(t, tc.schedule, got.Schedule)
			require.Equal(t, state.Meta, got.Meta)
			require.Zero(t, got.RunStartedAt)
			require.Zero(t, got.RunLeaseUntil, "a finished run's lease must not outlive it")
			require.Empty(t, got.RunLeaseID)
			require.Zero(t, got.Failures)
			require.Equal(t, int64(100), got.LastRunAt)
			ok, err = store.FinishRun(t.Context(), tc.name, "owner", result)
			require.NoError(t, err)
			require.False(t, ok, "a run can finish only once")
		})
	}
	// A one-shot task re-registered for another occurrence while the run
	// executed keeps that occurrence: the run did not execute it.
	for _, tc := range []struct {
		name       string
		status     scheduler.TaskStatus
		wantStatus scheduler.TaskStatus
	}{
		{"one_shot_rescheduled", scheduler.TaskStatusRunning, scheduler.TaskStatusActive},
		{"one_shot_rescheduled_paused", scheduler.TaskStatusPaused, scheduler.TaskStatusPaused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, store.UpsertTask(t.Context(), &scheduler.TaskState{
				TaskSummary: scheduler.TaskSummary{ID: tc.name, Status: tc.status, NextRunAt: 500, OneShot: true},
				LastRunID:   "owner", RunStartedAt: 100, RunAt: 500,
			}))
			executed := scheduler.RunResult{StartedAt: 100, EndedAt: 110, RunAt: 90, Success: true}
			ok, err := store.FinishRun(t.Context(), tc.name, "owner", executed)
			require.NoError(t, err)
			require.True(t, ok)
			got, err := store.GetTask(t.Context(), tc.name)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, int64(500), got.NextRunAt, "the newly registered occurrence must be kept")
			require.Equal(t, int64(500), got.RunAt)
			require.Zero(t, got.RunStartedAt)
			require.Equal(t, int64(100), got.LastRunAt)
		})
	}
	t.Run("failed_run", func(t *testing.T) {
		t.Parallel()
		state := &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{ID: "failed", Status: scheduler.TaskStatusRunning, Failures: 2},
			LastRunID:   "owner", RunStartedAt: 100,
		}
		require.NoError(t, store.UpsertTask(t.Context(), state))
		failure := result
		failure.Success = false
		ok, err := store.FinishRun(t.Context(), state.ID, "owner", failure)
		require.NoError(t, err)
		require.True(t, ok)
		got, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		require.Equal(t, int32(3), got.Failures)
		ok, err = store.FinishRun(t.Context(), state.ID, "owner", failure)
		require.NoError(t, err)
		require.False(t, ok)
	})
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		ok, err := store.FinishRun(t.Context(), "missing", "owner", result)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// BenchmarkFinishRun measures the conditional write with a fresh claim per run.
//
//nolint:mnd // Fixed timestamps keep the benchmark independent of the wall clock.
func BenchmarkFinishRun(b *testing.B, store scheduler.Storage) {
	b.Helper()
	state := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "bench", Status: scheduler.TaskStatusRunning},
		LastRunID:   "owner", RunStartedAt: 100,
	}
	result := scheduler.RunResult{StartedAt: 100, EndedAt: 110, Success: true}
	for b.Loop() {
		b.StopTimer()
		require.NoError(b, store.UpsertTask(b.Context(), state))
		b.StartTimer()
		ok, err := store.FinishRun(b.Context(), state.ID, "owner", result)
		require.NoError(b, err)
		require.True(b, ok)
	}
}
