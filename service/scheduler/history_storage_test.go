// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// recordingHistory is a separate history backend that records the cleanups
// and deletions the scheduler asks of it.
type recordingHistory struct {
	scheduler.HistoryStorage

	mu         sync.Mutex
	retentions []time.Duration
	deleted    []string
}

func (r *recordingHistory) CleanupHistory(ctx context.Context, retention time.Duration) error {
	r.mu.Lock()
	r.retentions = append(r.retentions, retention)
	r.mu.Unlock()
	return r.HistoryStorage.CleanupHistory(ctx, retention)
}

func (r *recordingHistory) DeleteHistory(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *recordingHistory) snapshot() (retentions []time.Duration, deleted []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.retentions), slices.Clone(r.deleted)
}

func TestScheduler_WithHistoryStorage(t *testing.T) {
	t.Parallel()
	tasks := mustNewMemory(t, 100)
	history := &recordingHistory{HistoryStorage: mustNewMemory(t, 100)}
	s := scheduler.New(tasks,
		scheduler.WithTickInterval(20*time.Millisecond),
		scheduler.WithCleanupInterval(20*time.Millisecond),
		scheduler.WithHistoryRetention(time.Hour),
		scheduler.WithHistoryStorage(history),
	)
	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	const id = "separate-history"
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: id, Schedule: "@every 1h", RunOnStart: true,
		Func: func(context.Context) error { return nil },
	}))
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		page, err := s.HistoryPaginated(ctx, id, scheduler.PageRequest{}, "success")
		return err == nil && len(page.Items) == 1
	}, "the run must be recorded in the history storage")

	for _, err := range tasks.History(ctx, id) {
		require.NoError(t, err)
		require.Fail(t, "history must not be written to the task storage")
	}
	var listed []string
	for h, err := range s.History(ctx, id) {
		require.NoError(t, err)
		listed = append(listed, h.TaskID)
	}
	require.Equal(t, []string{id}, listed)

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		retentions, _ := history.snapshot()
		return len(retentions) > 0
	}, "cleanup must reach the history storage")
	retentions, _ := history.snapshot()
	require.Equal(t, time.Hour, retentions[0])

	require.NoError(t, s.Unregister(ctx, id))
	_, deleted := history.snapshot()
	require.Equal(t, []string{id}, deleted, "Unregister must delete the history kept apart")
}

// stalledHistory is a history backend whose writes and deletes block until
// their context ends, and whose deletes then fail.
type stalledHistory struct {
	scheduler.HistoryStorage
}

func (stalledHistory) AddHistory(ctx context.Context, _ *scheduler.TaskHistory) error {
	<-ctx.Done()
	return ctx.Err()
}

func (stalledHistory) DeleteHistory(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

// deadlineStorage is a task storage whose bookkeeping reads and writes honor
// their context's deadline, which the in-memory storage ignores.
type deadlineStorage struct {
	*memory.Storage
}

func (d deadlineStorage) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d.Storage.GetTask(ctx, id)
}

func (d deadlineStorage) FinishRun(ctx context.Context, id, runID string, result scheduler.RunResult) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return d.Storage.FinishRun(ctx, id, runID, result)
}

// A stalled history backend must neither keep a finished run in
// TaskStatusRunning nor hold Unregister beyond the storage timeout, and a
// failed history delete must not fail Unregister.
func TestScheduler_StalledHistoryStorage(t *testing.T) {
	t.Parallel()
	tasks := deadlineStorage{Storage: mustNewMemory(t, 100)}
	s := scheduler.New(tasks,
		scheduler.WithTickInterval(20*time.Millisecond),
		scheduler.WithStorageTimeout(100*time.Millisecond),
		scheduler.WithHistoryStorage(stalledHistory{HistoryStorage: mustNewMemory(t, 100)}),
	)
	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	const id = "stalled-history"
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: id, Schedule: "@every 1h", RunOnStart: true,
		Func: func(context.Context) error { return nil },
	}))
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		st, err := tasks.GetTask(ctx, id)
		return err == nil && st != nil && st.LastRunAt > 0 && st.Status == scheduler.TaskStatusActive
	}, "the run must finish although recording its history timed out")

	start := time.Now()
	require.NoError(t, s.Unregister(ctx, id), "a failed history delete must not fail Unregister")
	require.Less(t, time.Since(start), 2*time.Second, "the history delete must be bounded by the storage timeout")
	st, err := tasks.GetTask(ctx, id)
	require.NoError(t, err)
	require.Nil(t, st)
}

func TestScheduler_HistoryInStorageByDefault(t *testing.T) {
	t.Parallel()
	tasks := mustNewMemory(t, 100)
	s := scheduler.New(tasks, scheduler.WithTickInterval(20*time.Millisecond))
	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	const id = "default-history"
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: id, Schedule: "@every 1h", RunOnStart: true,
		Func: func(context.Context) error { return nil },
	}))
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		for h, err := range tasks.History(ctx, id) {
			return err == nil && h.TaskID == id
		}
		return false
	}, "without WithHistoryStorage the run must be recorded in the task storage")
}
