// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// TestIntegration_MongoDueTasks exercises the DueTasks predicate against a live
// MongoDB, where it is a server-side query rather than a Go filter. The memory
// backend cannot catch a mistranslated filter, and a wrong predicate here is
// silent from the scheduler's side: too narrow and tasks quietly stop firing,
// too wide and they fire early.
func TestIntegration_MongoDueTasks(t *testing.T) {
	t.Parallel()

	const now int64 = 1000

	s := newClaimIT(t)
	ctx := t.Context()

	seed := []struct {
		id        string
		status    scheduler.TaskStatus
		nextRunAt int64
	}{
		{"a-due", scheduler.TaskStatusActive, now - 100},
		{"b-due", scheduler.TaskStatusActive, now - 1},
		{"c-exactly-now", scheduler.TaskStatusActive, now},
		{"d-future", scheduler.TaskStatusActive, now + 1},
		{"e-paused", scheduler.TaskStatusPaused, now - 1},
		{"f-disabled", scheduler.TaskStatusDisabled, now - 1},
		{"g-completed", scheduler.TaskStatusCompleted, now - 1},
		{"h-running", scheduler.TaskStatusRunning, now - 1},
	}
	for _, sd := range seed {
		require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{ID: sd.id, Status: sd.status, NextRunAt: sd.nextRunAt},
		}))
	}

	var states []*scheduler.TaskState
	for state, err := range s.DueTasks(ctx, now) {
		require.NoError(t, err)
		states = append(states, state)
	}

	got := make([]string, 0, len(states))
	for _, st := range states {
		got = append(got, st.ID)
	}
	require.Equal(t, []string{"a-due", "b-due", "c-exactly-now"}, got,
		"boundary is inclusive: NextRunAt == now is due")

	// next_run_at carries omitempty, so a task whose NextRunAt is zero has no
	// such field in the document at all. Assert the payload survives the
	// document→state conversion rather than trusting the ID set alone.
	for _, st := range states {
		require.Equalf(t, scheduler.TaskStatusActive, st.Status, "task %q lost its status in conversion", st.ID)
		require.LessOrEqualf(t, st.NextRunAt, now, "task %q lost its NextRunAt in conversion", st.ID)
	}
}
