// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// TestRunOnStartForStoredTask pins RunOnStart for a task that already exists
// in storage — a restarted instance registering it again: it is due at once
// unless it started within the grace window.
func TestRunOnStartForStoredTask(t *testing.T) {
	t.Parallel()

	const schedule = "@every 1h"
	tests := []struct {
		name       string
		lastRun    time.Duration // before now; 0 means never ran
		inFlight   bool          // the recent start is a run still in progress
		grace      time.Duration
		runOnStart bool
		wantNow    bool
	}{
		{name: "never ran", runOnStart: true, grace: scheduler.DefaultRunOnStartGrace, wantNow: true},
		{name: "ran long ago", lastRun: 30 * time.Minute, runOnStart: true, grace: scheduler.DefaultRunOnStartGrace, wantNow: true},
		{name: "ran within grace", lastRun: time.Minute, runOnStart: true, grace: scheduler.DefaultRunOnStartGrace},
		{name: "run in flight within grace", lastRun: time.Minute, inFlight: true, runOnStart: true, grace: scheduler.DefaultRunOnStartGrace},
		{name: "minimal grace runs on every start", lastRun: time.Minute, runOnStart: true, grace: time.Nanosecond, wantNow: true},
		{name: "flag off keeps the stored schedule", lastRun: 30 * time.Minute, grace: scheduler.DefaultRunOnStartGrace},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			storage := mustNewMemory(t, 10)
			now := time.Now()
			stored := now.Add(time.Hour).Unix()

			state := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
				ID: "task", Status: scheduler.TaskStatusActive, Priority: corescheduler.TaskPriorityNormal,
				Schedule: schedule, NextRunAt: stored,
			}}
			if tc.lastRun > 0 {
				started := now.Add(-tc.lastRun).Unix()
				if tc.inFlight {
					state.RunStartedAt = started
				} else {
					state.LastRunAt = started
				}
			}
			created, err := storage.CreateTask(ctx, state)
			require.NoError(t, err)
			require.True(t, created)

			s := scheduler.New(storage, scheduler.WithRunOnStartGrace(tc.grace))
			require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
				ID: "task", Schedule: schedule, RunOnStart: tc.runOnStart,
				Func: func(context.Context) error { return nil },
			}))

			got, err := storage.GetTask(ctx, "task")
			require.NoError(t, err)
			if tc.wantNow {
				require.LessOrEqual(t, got.NextRunAt, time.Now().Unix(), "due at once")
			} else {
				require.Equal(t, stored, got.NextRunAt, "stored schedule kept")
			}
		})
	}
}
