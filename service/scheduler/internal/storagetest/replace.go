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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := running("replace_" + tc.name)
			require.NoError(t, store.UpsertTask(t.Context(), state))

			fence := scheduler.FenceOf(state)
			tc.mutate(&fence)
			ok, err := store.ReplaceTaskIf(t.Context(), reset(state), fence)
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

		next := *state
		next.Status = scheduler.TaskStatusCompleted
		ok, err := store.ReplaceTaskIf(t.Context(), &next, scheduler.FenceOf(state))
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		state := running("replace_missing")
		ok, err := store.ReplaceTaskIf(t.Context(), state, scheduler.FenceOf(state))
		require.NoError(t, err)
		require.False(t, ok)
	})
}
