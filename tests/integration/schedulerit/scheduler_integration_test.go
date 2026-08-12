// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// TestDispatchRoundTrip is the baseline: a registered task runs, and the state
// the scheduler wrote survives the trip through a real driver.
//
// The field assertions are the point. Reading a task back is a document
// decode, and when that decode silently dropped the embedded summary the task
// came back with a zero status — dispatch and stale recovery both stopped
// seeing it, with no error anywhere. Asserting only "the function ran" would
// not have noticed.
func TestDispatchRoundTrip(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			s := startScheduler(t, storage)
			ctx := t.Context()

			var runs atomic.Int32
			require.NoError(t, s.Register(ctx, countingTask("round-trip", &runs)))

			require.Eventually(t, func() bool { return runs.Load() >= 1 },
				settleWindow, samplingInterval, "task never ran")

			// The run has to be reflected in durable state, not just in memory.
			require.Eventually(t, func() bool {
				st, err := storage.GetTask(ctx, "round-trip")
				return err == nil && st != nil && st.LastRunAt > 0
			}, settleWindow, samplingInterval, "the run was never recorded in storage")

			st, err := storage.GetTask(ctx, "round-trip")
			require.NoError(t, err)
			require.NotNil(t, st)

			require.Equal(t, "round-trip", st.ID)
			require.Equal(t, scheduler.TaskStatusActive, st.Status,
				"a finished run must leave the task active, not stranded in running")
			require.Equal(t, "@every 1h", st.Schedule, "the schedule must survive the round trip")
			require.Zero(t, st.RunStartedAt, "RunStartedAt must be cleared once the run ends")
			require.Zero(t, st.Failures, "a successful run must reset the failure count")
			require.Greater(t, st.NextRunAt, time.Now().Unix(),
				"the next occurrence must be scheduled into the future")

			var entries int
			for h, err := range storage.History(ctx, "round-trip") {
				require.NoError(t, err)
				require.True(t, h.Success, "a successful run must be recorded as such")
				require.Equal(t, "round-trip", h.TaskID)
				entries++
			}
			require.GreaterOrEqual(t, entries, 1, "the run must appear in history")
		})
	}
}

// TestAtMostOnceAcrossInstances is the property the whole design rests on: two
// schedulers sharing one storage must not both execute the same occurrence.
//
// Neither instance has a leader elector here, so both consider themselves
// entitled to dispatch and both will. What stops a double execution is
// Storage.ClaimRun — a single atomic compare-and-swap, fenced on the
// occurrence's next-run time, that only one caller can win. That arbitration
// happens on the server, so it is exactly what an in-memory store cannot
// demonstrate.
func TestAtMostOnceAcrossInstances(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()

			// One counter shared by both instances: the assertion is about the
			// total across the cluster, not about which node won.
			var runs atomic.Int32

			first := startScheduler(t, storage)
			second := startScheduler(t, storage)

			require.NoError(t, first.Register(ctx, countingTask("contended", &runs)))
			require.NoError(t, second.Register(ctx, countingTask("contended", &runs)))

			require.Eventually(t, func() bool { return runs.Load() >= 1 },
				settleWindow, samplingInterval, "neither instance ran the task")

			// Give the loser every chance to run it a second time.
			require.Never(t, func() bool { return runs.Load() > 1 },
				quietWindow, samplingInterval,
				"one occurrence was executed more than once: the claim did not arbitrate")
		})
	}
}

// TestRecoversTaskStrandedByCrash covers the state a killed process leaves
// behind.
//
// A task is marked running and then abandoned, which is what a SIGKILL between
// the claim and the result write produces. Startup recovery must reclaim it:
// without that the occurrence is lost permanently, because a running task is
// not a candidate for dispatch and nothing else ever revisits it. The seeded
// state has to be durable for the next process to find it at all, which is why
// this cannot be shown in memory.
func TestRecoversTaskStrandedByCrash(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()

			const taskID = "stranded"
			crashedAt := time.Now().Add(-time.Hour).Unix()

			require.NoError(t, storage.UpsertTask(ctx, &scheduler.TaskState{
				TaskSummary: scheduler.TaskSummary{
					ID:        taskID,
					Status:    scheduler.TaskStatusRunning,
					Schedule:  "@every 1h",
					NextRunAt: crashedAt,
				},
				RunStartedAt: crashedAt,
				UpdatedAt:    crashedAt,
			}))

			// Starting a scheduler stands in for the replacement process.
			startScheduler(t, storage)

			require.Eventually(t, func() bool {
				st, err := storage.GetTask(ctx, taskID)
				return err == nil && st != nil && st.Status == scheduler.TaskStatusActive
			}, settleWindow, samplingInterval, "the stranded task was never reclaimed")

			st, err := storage.GetTask(ctx, taskID)
			require.NoError(t, err)
			require.Zero(t, st.RunStartedAt, "recovery must clear the abandoned run marker")
			require.Positive(t, st.Failures, "an abandoned run must count as a failure")
		})
	}
}

// TestFollowerDispatchesNothing checks that leadership actually gates dispatch.
//
// Leadership is an optimization rather than a correctness boundary — ClaimRun
// is what makes double execution impossible — but it is the mechanism that
// keeps every replica in a deployment from stampeding the same backend, so a
// follower that quietly dispatches anyway defeats the purpose.
func TestFollowerDispatchesNothing(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()

			elector := &stubElector{}
			s := startScheduler(t, storage, scheduler.WithLeaderElector(elector))

			var runs atomic.Int32
			require.NoError(t, s.Register(ctx, countingTask("gated", &runs)))

			require.Never(t, func() bool { return runs.Load() > 0 },
				quietWindow, samplingInterval, "a follower dispatched a task")

			elector.leader.Store(true)

			require.Eventually(t, func() bool { return runs.Load() >= 1 },
				settleWindow, samplingInterval, "the task never ran after taking leadership")
		})
	}
}

// TestFailedRunIsRecorded checks the unhappy path end to end: the error reaches
// history and the failure count, and the task stays schedulable.
func TestFailedRunIsRecorded(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			s := startScheduler(t, storage)
			ctx := t.Context()

			const taskID = "failing"
			var runs atomic.Int32
			require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
				ID:         taskID,
				Schedule:   "@every 1h",
				RunOnStart: true,
				Func: func(context.Context) error {
					runs.Add(1)
					return context.DeadlineExceeded
				},
			}))

			require.Eventually(t, func() bool {
				st, err := storage.GetTask(ctx, taskID)
				return err == nil && st != nil && st.Failures > 0
			}, settleWindow, samplingInterval, "the failure was never recorded")

			st, err := storage.GetTask(ctx, taskID)
			require.NoError(t, err)
			require.Equal(t, scheduler.TaskStatusActive, st.Status,
				"a failed run must leave the task schedulable, not stranded")

			var sawFailure bool
			for h, err := range storage.History(ctx, taskID) {
				require.NoError(t, err)
				if !h.Success {
					require.NotEmpty(t, h.Error, "a failed run must record why")
					sawFailure = true
				}
			}
			require.True(t, sawFailure, "the failure must appear in history")
		})
	}
}
