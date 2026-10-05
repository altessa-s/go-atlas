// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// TestConcurrentRegistrationRunsOnce registers the same new tasks on two
// instances at the same time. Registration creates a task with an atomic
// insert-if-absent, so the instance that loses the race merges into the state
// the winner wrote — possibly already claimed — instead of overwriting it with
// a fresh, claimable one. Every task must run exactly once.
func TestConcurrentRegistrationRunsOnce(t *testing.T) {
	t.Parallel()

	const tasks = 20

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()
			instances := []*scheduler.Scheduler{startScheduler(t, storage), startScheduler(t, storage)}

			runs := make([]atomic.Int32, tasks)
			var wg sync.WaitGroup
			for _, s := range instances {
				for i := range tasks {
					wg.Go(func() {
						require.NoError(t, s.Register(ctx, countingTask(fmt.Sprintf("concurrent-%02d", i), &runs[i])))
					})
				}
			}
			wg.Wait()

			require.Eventually(t, func() bool {
				for i := range runs {
					if runs[i].Load() < 1 {
						return false
					}
				}
				return true
			}, settleWindow, samplingInterval, "not every task ran")

			require.Never(t, func() bool {
				for i := range runs {
					if runs[i].Load() > 1 {
						return true
					}
				}
				return false
			}, quietWindow, samplingInterval, "a task registered concurrently ran more than once")
		})
	}
}

// TestLongRunSurvivesOtherInstanceRecovery runs a one-shot task for longer than
// the stale-task timeout while a second instance — started after the claim, so
// its startup recovery sees the run — keeps running stale recovery. The owner
// persists and renews the run's lease, so the second instance must neither
// reset nor re-execute the task.
func TestLongRunSurvivesOtherInstanceRecovery(t *testing.T) {
	t.Parallel()

	const busy = 7 * time.Second

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()

			var runs atomic.Int32
			cfg := corescheduler.TaskConfig{
				ID:    "long-one-shot",
				RunAt: time.Now(),
				Func: func(ctx context.Context) error {
					runs.Add(1)
					select {
					case <-time.After(busy):
					case <-ctx.Done():
					}
					return nil
				},
			}

			owner := startScheduler(t, storage, scheduler.WithStaleTaskTimeout(time.Second))
			require.NoError(t, owner.Register(ctx, cfg))
			require.Eventually(t, func() bool { return runs.Load() == 1 },
				settleWindow, samplingInterval, "the task never started")

			other := startScheduler(t, storage, scheduler.WithStaleTaskTimeout(time.Second))
			require.NoError(t, other.Register(ctx, cfg))

			require.Never(t, func() bool {
				st, err := storage.GetTask(ctx, cfg.ID)
				return runs.Load() > 1 || err != nil || st == nil || st.Failures != 0
			}, busy-time.Second, samplingInterval, "a live run was recovered or executed twice")

			require.Eventually(t, func() bool {
				st, err := storage.GetTask(ctx, cfg.ID)
				return err == nil && st != nil && st.Status == scheduler.TaskStatusCompleted
			}, settleWindow, samplingInterval, "the one-shot task never completed")
			require.Equal(t, int32(1), runs.Load())
		})
	}
}

// TestCompletedOneShotStaysCompleted runs a one-shot task that is paused and
// resumed while it executes, then registers it again on a second instance with
// the same RunAt, as a restart or a scaled-out deployment would. The finished
// run completes the task despite the interleaved pause and resume, and the
// persisted RunAt keeps the re-registration from running it again.
func TestCompletedOneShotStaysCompleted(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			storage := b.storage(t)
			ctx := t.Context()

			var runs atomic.Int32
			release := make(chan struct{})
			cfg := corescheduler.TaskConfig{
				ID:    "one-shot",
				RunAt: time.Now().Add(-time.Minute),
				Func: func(ctx context.Context) error {
					runs.Add(1)
					select {
					case <-release:
					case <-ctx.Done():
					}
					return nil
				},
			}

			first := startScheduler(t, storage)
			require.NoError(t, first.Register(ctx, cfg))
			require.Eventually(t, func() bool { return runs.Load() == 1 },
				settleWindow, samplingInterval, "the task never started")

			require.NoError(t, first.PauseTask(ctx, cfg.ID))
			require.NoError(t, first.ResumeTask(ctx, cfg.ID))
			close(release)

			completed := func() bool {
				st, err := storage.GetTask(ctx, cfg.ID)
				return err == nil && st != nil && st.Status == scheduler.TaskStatusCompleted
			}
			require.Eventually(t, completed, settleWindow, samplingInterval, "the one-shot task never completed")

			second := startScheduler(t, storage)
			require.NoError(t, second.Register(ctx, cfg))
			require.True(t, completed(), "re-registering with the same RunAt must keep the task completed")

			require.Never(t, func() bool { return runs.Load() > 1 }, quietWindow, samplingInterval,
				"a completed one-shot task ran again")
		})
	}
}
