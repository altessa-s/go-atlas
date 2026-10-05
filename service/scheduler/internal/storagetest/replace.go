// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// ReplaceTaskIf verifies that a fenced replace succeeds only while every fence
// field still matches, including zero-valued (absent) fields. The supplied
// store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func ReplaceTaskIf(t *testing.T, store scheduler.Storage) {
	t.Helper()
	running := func(id string) *scheduler.TaskState {
		return &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: scheduler.TaskStatusRunning, Schedule: "@every 1m", NextRunAt: 300,
		}, LastRunID: "owner", RunStartedAt: 100}
	}
	reset := func(state *scheduler.TaskState) *scheduler.TaskState {
		next := *state
		next.Status = scheduler.TaskStatusActive
		next.RunStartedAt = 0
		next.Failures = 1
		return &next
	}

	for _, tc := range []struct {
		name    string
		mutate  func(*scheduler.TaskFence)
		wantHit bool
	}{
		{"match", func(*scheduler.TaskFence) {}, true},
		{"status_changed", func(f *scheduler.TaskFence) { f.Status = scheduler.TaskStatusActive }, false},
		{"next_run_changed", func(f *scheduler.TaskFence) { f.NextRunAt = 301 }, false},
		{"run_id_changed", func(f *scheduler.TaskFence) { f.LastRunID = "other" }, false},
		{"run_started_changed", func(f *scheduler.TaskFence) { f.RunStartedAt = 101 }, false},
		{"revision_changed", func(f *scheduler.TaskFence) { f.Revision++ }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := running("replace_" + tc.name)
			require.NoError(t, store.UpsertTask(t.Context(), state))

			stored, err := store.GetTask(t.Context(), state.ID)
			require.NoError(t, err)
			fence := scheduler.FenceOf(stored)
			tc.mutate(&fence)
			ok, err := store.ReplaceTaskIf(t.Context(), reset(stored), fence)
			require.NoError(t, err)
			require.Equal(t, tc.wantHit, ok)

			got, err := store.GetTask(t.Context(), state.ID)
			require.NoError(t, err)
			if tc.wantHit {
				require.Equal(t, scheduler.TaskStatusActive, got.Status)
				require.Zero(t, got.RunStartedAt)
				require.Equal(t, int32(1), got.Failures)
			} else {
				require.Equal(t, scheduler.TaskStatusRunning, got.Status)
				require.Equal(t, int64(100), got.RunStartedAt)
			}
		})
	}

	t.Run("zero_fields", func(t *testing.T) {
		t.Parallel()
		state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: "replace_zero", Status: scheduler.TaskStatusActive, OneShot: true,
		}}
		require.NoError(t, store.UpsertTask(t.Context(), state))

		stored, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		next := *stored
		next.Status = scheduler.TaskStatusCompleted
		ok, err := store.ReplaceTaskIf(t.Context(), &next, scheduler.FenceOf(stored))
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("concurrent_upsert", func(t *testing.T) {
		t.Parallel()
		state := running("replace_concurrent_upsert")
		require.NoError(t, store.UpsertTask(t.Context(), state))
		stored, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		fence := scheduler.FenceOf(stored)

		// A write that leaves every run-ownership field untouched.
		changed := *stored
		changed.Meta = map[string]string{"keep": "value"}
		require.NoError(t, store.UpsertTask(t.Context(), &changed))

		ok, err := store.ReplaceTaskIf(t.Context(), reset(stored), fence)
		require.NoError(t, err)
		require.False(t, ok)
		got, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		require.Equal(t, "value", got.Meta["keep"])
	})

	t.Run("revision_increments", func(t *testing.T) {
		t.Parallel()
		state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: "replace_revision", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
		}, Revision: 99} // caller-supplied revisions are ignored
		rev := func() int64 {
			got, err := store.GetTask(t.Context(), state.ID)
			require.NoError(t, err)
			return got.Revision
		}
		require.NoError(t, store.UpsertTask(t.Context(), state))
		require.Equal(t, int64(1), rev())
		require.NoError(t, store.UpsertTask(t.Context(), state))
		require.Equal(t, int64(2), rev())

		ok, err := store.ClaimRun(t.Context(), state.ID, scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: "owner"})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, int64(3), rev())

		ok, err = store.FinishRun(t.Context(), state.ID, "owner",
			scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 400, Schedule: "@every 1m", Success: true})
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, int64(4), rev())

		stored, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		ok, err = store.ReplaceTaskIf(t.Context(), stored, scheduler.FenceOf(stored))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, int64(5), rev())
	})

	t.Run("dollar_strings_round_trip", func(t *testing.T) {
		t.Parallel()
		state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: "replace_dollar", Status: scheduler.TaskStatusActive, Description: "$status",
		}, Meta: map[string]string{"k": "$revision"}}
		require.NoError(t, store.UpsertTask(t.Context(), state))
		got, err := store.GetTask(t.Context(), state.ID)
		require.NoError(t, err)
		require.Equal(t, "$status", got.Description)
		require.Equal(t, "$revision", got.Meta["k"])
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		state := running("replace_missing")
		ok, err := store.ReplaceTaskIf(t.Context(), state, scheduler.FenceOf(state))
		require.NoError(t, err)
		require.False(t, ok)
	})
}
