// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// takeoverOnRecoveryRead finishes the stale run and lets a new run claim the
// task right before stale recovery re-reads it, reproducing the window between
// the collection pass and the reset write.
type takeoverOnRecoveryRead struct {
	*memory.Storage
	once sync.Once
}

func (s *takeoverOnRecoveryRead) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	var err error
	s.once.Do(func() {
		var state *scheduler.TaskState
		if state, err = s.Storage.GetTask(ctx, id); err != nil {
			return
		}
		if _, err = s.Storage.FinishRun(ctx, id, state.LastRunID, scheduler.RunResult{
			StartedAt: state.RunStartedAt, EndedAt: time.Now().Unix(), NextRunAt: state.NextRunAt, Schedule: state.Schedule, Success: true,
		}); err != nil {
			return
		}
		_, err = s.Storage.ClaimRun(ctx, id, state.NextRunAt, time.Now().Unix(), "new-owner")
	})
	if err != nil {
		return nil, err
	}
	return s.Storage.GetTask(ctx, id)
}

func TestStaleRecoveryCannotOverwriteConcurrentClaim(t *testing.T) {
	t.Parallel()
	mem, err := memory.New(10)
	require.NoError(t, err)
	require.NoError(t, mem.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary:  scheduler.TaskSummary{ID: "task", Status: scheduler.TaskStatusRunning, Schedule: "@every 1h", NextRunAt: 1},
		LastRunID:    "stale-owner",
		RunStartedAt: 1,
	}))

	s := scheduler.New(&takeoverOnRecoveryRead{Storage: mem}, scheduler.WithTickInterval(time.Hour))
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
	})

	state, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "new-owner", state.LastRunID)
	require.Equal(t, scheduler.TaskStatusRunning, state.Status)
	require.NotZero(t, state.RunStartedAt)
	require.Zero(t, state.Failures)
}

// pauseOnSkipRead pauses the task right before applySkip re-reads it,
// reproducing a PauseTask that lands between the due scan and the skip write.
type pauseOnSkipRead struct {
	*memory.Storage
	armed atomic.Bool
	fired chan struct{}
}

func (s *pauseOnSkipRead) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	if s.armed.CompareAndSwap(true, false) {
		state, err := s.Storage.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		state.Status = scheduler.TaskStatusPaused
		if err := s.Storage.UpsertTask(ctx, state); err != nil {
			return nil, err
		}
		close(s.fired)
	}
	return s.Storage.GetTask(ctx, id)
}

func TestSkipCannotOverwriteConcurrentPause(t *testing.T) {
	t.Parallel()
	mem, err := memory.New(10)
	require.NoError(t, err)
	store := &pauseOnSkipRead{Storage: mem, fired: make(chan struct{})}

	s := scheduler.New(store, scheduler.WithTickInterval(20*time.Millisecond))
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
	})
	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{
		ID: "task", Schedule: "@every 1h", Func: func(context.Context) error { return nil },
	}))

	state, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	state.SkipNextRun = true
	state.NextRunAt = 1 // due now
	require.NoError(t, mem.UpsertTask(t.Context(), state))
	store.armed.Store(true)

	select {
	case <-store.fired:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "skip was not attempted")
	}
	// Let the in-flight skip finish its (rejected) write.
	require.Never(t, func() bool {
		got, err := mem.GetTask(t.Context(), "task")
		return err != nil || !got.SkipNextRun || got.NextRunAt != 1
	}, 200*time.Millisecond, 10*time.Millisecond)

	got, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusPaused, got.Status)
}
