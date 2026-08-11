// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// ctxAwareStorage rejects calls made with an already-canceled context, the way
// a networked backend does. The in-memory storage ignores its context
// entirely, so it cannot express a cancellation bug on its own.
type ctxAwareStorage struct {
	*memory.Storage
}

func (c *ctxAwareStorage) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.Storage.GetTask(ctx, id)
}

func (c *ctxAwareStorage) UpsertTask(ctx context.Context, state *scheduler.TaskState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Storage.UpsertTask(ctx, state)
}

func (c *ctxAwareStorage) AddHistory(ctx context.Context, h *scheduler.TaskHistory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Storage.AddHistory(ctx, h)
}

// hangingStorage blocks every listing call until its context is done once hang
// is set, simulating a wedged backend that never answers. Both listing methods
// are covered: the tick reads through DueTasks, stale recovery through Tasks.
type hangingStorage struct {
	*memory.Storage
	hang atomic.Bool
}

func (h *hangingStorage) Tasks(ctx context.Context) iter.Seq2[*scheduler.TaskState, error] {
	if !h.hang.Load() {
		return h.Storage.Tasks(ctx)
	}
	return blockUntilDone[*scheduler.TaskState](ctx)
}

func (h *hangingStorage) DueTasks(ctx context.Context, now int64) iter.Seq2[*scheduler.TaskState, error] {
	if !h.hang.Load() {
		return h.Storage.DueTasks(ctx, now)
	}
	return blockUntilDone[*scheduler.TaskState](ctx)
}

// blockUntilDone yields nothing until ctx is done, then reports its error.
func blockUntilDone[T any](ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		<-ctx.Done()
		var zero T
		yield(zero, ctx.Err())
	}
}

// TestScheduler_Stop_PersistsFinalState asserts that a task in flight when Stop
// is called still has its result written back.
//
// Stop cancels the lifecycle context and only then waits on the WaitGroup. If
// the post-run bookkeeping inherited that cancellation, every graceful
// shutdown would strand its running tasks in TaskStatusRunning, and periodic
// stale recovery would later resurrect them with an inflated failure count.
func TestScheduler_Stop_PersistsFinalState(t *testing.T) {
	t.Parallel()

	storage := &ctxAwareStorage{Storage: mustNewMemory(t, 100)}
	s := scheduler.New(storage, scheduler.WithTickInterval(20*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))

	var once sync.Once
	started := make(chan struct{})
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "long-running",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(taskCtx context.Context) error {
			once.Do(func() { close(started) })
			// Released when Stop cancels the lifecycle context.
			<-taskCtx.Done()
			return nil
		},
	}))

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task never started")
	}

	detached := context.WithoutCancel(ctx)
	stopCtx, cancel := context.WithTimeout(detached, 2*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(stopCtx))

	state, err := storage.GetTask(detached, "long-running")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, scheduler.TaskStatusActive, state.Status,
		"task interrupted by Stop must be written back as active, not left running")
	require.Zero(t, state.RunStartedAt,
		"RunStartedAt must be cleared so stale recovery does not resurrect the run")
}

// TestScheduler_StorageTimeout_TickLoopRecovers asserts that a wedged storage
// backend cannot stall the main loop permanently.
//
// The loop is a single goroutine, and its storage calls ride the lifecycle
// context, which carries no deadline of its own. Without the per-operation cap
// from WithStorageTimeout, one unanswered Tasks call parks the loop until Stop
// and no task is ever dispatched again — even after the backend recovers.
func TestScheduler_StorageTimeout_TickLoopRecovers(t *testing.T) {
	t.Parallel()

	storage := &hangingStorage{Storage: mustNewMemory(t, 100)}
	s := scheduler.New(storage,
		scheduler.WithTickInterval(20*time.Millisecond),
		scheduler.WithStorageTimeout(100*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	storage.hang.Store(true)

	var runs atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "recovers",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	}))

	// Several tick intervals elapse while every Tasks call hangs.
	time.Sleep(300 * time.Millisecond)
	require.Zero(t, runs.Load(), "task must not run while the storage is wedged")

	storage.hang.Store(false)

	require.Eventually(t, func() bool { return runs.Load() > 0 },
		2*time.Second, 20*time.Millisecond,
		"tick loop did not resume after the storage recovered")
}

// TestScheduler_StaticSemaphore_DoesNotAccumulateGoroutines asserts that a task
// starved of a concurrency slot costs one blocked goroutine, not one per tick.
//
// In static semaphore mode the dispatch goroutine blocks on the semaphore
// before it reaches executeTask, so it sets neither the storage status nor the
// running flag. Every subsequent tick therefore saw the task as still due and
// idle, and stacked another waiter onto it — growth bounded only by how long
// the contention lasted.
//
// Not parallel: it reads runtime.NumGoroutine(), which is process-global and
// would be perturbed by any test running alongside it.
func TestScheduler_StaticSemaphore_DoesNotAccumulateGoroutines(t *testing.T) {
	const (
		tickInterval  = 20 * time.Millisecond
		observedTicks = 40
		// One waiter for the starved task plus headroom for runtime
		// bookkeeping. Per-tick stacking would add ~observedTicks.
		maxGrowth = 10
	)

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(tickInterval),
		scheduler.WithMaxConcurrentTasks(1),
		scheduler.WithReservedHighPrioritySlots(0))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))

	var once sync.Once
	hogRunning := make(chan struct{})
	release := make(chan struct{})

	// The hog must own the only slot before the starved task is ever
	// dispatched. Registering both up front would race: they are dispatched in
	// the same tick, and whichever goroutine reaches the semaphore first wins
	// it — an instant task would simply run and reschedule an hour out.
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "hog",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(context.Context) error {
			once.Do(func() { close(hogRunning) })
			<-release
			return nil
		},
	}))

	select {
	case <-hogRunning:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("hog task never started")
	}

	// The slot is now held for the rest of the test, so this task stays due
	// and undispatchable — every tick is an opportunity to stack a waiter.
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "starved",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func:       func(context.Context) error { return nil },
	}))

	// Let the starved task's first dispatch settle onto the semaphore.
	time.Sleep(5 * tickInterval)
	baseline := runtime.NumGoroutine()

	time.Sleep(observedTicks * tickInterval)
	growth := runtime.NumGoroutine() - baseline

	close(release)
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(stopCtx))

	require.Lessf(t, growth, maxGrowth,
		"goroutine count grew by %d over %d ticks; a starved task must hold one waiter, not one per tick",
		growth, observedTicks)
}

// TestScheduler_TriggerTask_RejectsSecondDispatch pins the single-dispatch-slot
// contract that bounds the blocked set: a manual trigger for a task that is
// already queued or running is refused rather than stacked behind it.
func TestScheduler_TriggerTask_RejectsSecondDispatch(t *testing.T) {
	t.Parallel()

	s, ctx := startScheduler(t)

	var once sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "single-slot",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(context.Context) error {
			once.Do(func() { close(started) })
			<-release
			return nil
		},
	}))

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task never started")
	}

	require.ErrorIs(t, s.TriggerTask(ctx, "single-slot"), scheduler.ErrTaskAlreadyDispatched)
}

// TestScheduler_ExecuteTask_DiscardsResultOfReclaimedRun asserts that a run
// whose task was taken over mid-flight does not write its result.
//
// Stale recovery can reset a long run to active, after which a competing
// instance claims the next occurrence. The original run is still executing. If
// it wrote its bookkeeping unconditionally it would stamp the *live* run as
// finished — status back to active, RunStartedAt cleared — which lets a third
// dispatch claim the task while the second is still running. LastRunID, set by
// ClaimRun, is the fence.
func TestScheduler_ExecuteTask_DiscardsResultOfReclaimedRun(t *testing.T) {
	t.Parallel()

	const (
		taskID         = "reclaimed"
		reclaimedRunID = "run-owned-by-another-instance"
	)

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage, scheduler.WithTickInterval(20*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	var ran atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         taskID,
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(context.Context) error {
			if ran.Add(1) > 1 {
				return nil
			}
			// Stand in for stale recovery resetting the task and a competing
			// instance winning the next claim while this run is still going.
			st, err := storage.GetTask(ctx, taskID)
			if err != nil || st == nil {
				return err
			}
			st.Status = scheduler.TaskStatusRunning
			st.LastRunID = reclaimedRunID
			st.RunStartedAt = time.Now().Unix()
			return storage.UpsertTask(ctx, st)
		},
	}))

	require.Eventually(t, func() bool { return ran.Load() > 0 },
		2*time.Second, 20*time.Millisecond, "task never ran")

	require.Never(t, func() bool {
		st, err := storage.GetTask(ctx, taskID)
		return err == nil && st != nil && st.LastRunID != reclaimedRunID
	}, time.Second, 20*time.Millisecond,
		"a reclaimed run must discard its result instead of overwriting the live run")
}
