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
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

type takeoverOnFinish struct {
	*memory.Storage
	done chan bool
}

func (s *takeoverOnFinish) FinishRun(ctx context.Context, id, runID string, result scheduler.RunResult) (bool, error) {
	state, err := s.Storage.GetTask(ctx, id)
	if err != nil {
		return false, err
	}
	// Stand in for stale recovery: it reactivates the task and clears the
	// unfinished run, which ClaimRun requires before another claim.
	state.Status = scheduler.TaskStatusActive
	state.RunStartedAt = 0
	if err = s.Storage.UpsertTask(ctx, state); err != nil {
		return false, err
	}
	if _, err = s.Storage.ClaimRun(ctx, id, scheduler.RunClaim{NextRunAt: state.NextRunAt, StartedAt: time.Now().Unix(), RunID: "new-owner"}); err != nil {
		return false, err
	}
	ok, err := s.Storage.FinishRun(ctx, id, runID, result)
	s.done <- ok
	return ok, err
}

func TestFinalizationCannotOverwriteConcurrentClaim(t *testing.T) {
	t.Parallel()
	mem, err := memory.New(10)
	require.NoError(t, err)
	store := &takeoverOnFinish{Storage: mem, done: make(chan bool, 1)}
	s := scheduler.New(store, scheduler.WithTickInterval(time.Hour))
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
	})
	require.NoError(t, s.Register(t.Context(), corescheduler.TaskConfig{ID: "task", Schedule: "@every 1h", Func: func(context.Context) error { return nil }}))
	require.NoError(t, s.TriggerTask(t.Context(), "task"))
	select {
	case applied := <-store.done:
		require.False(t, applied)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "task did not finish")
	}
	state, err := mem.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "new-owner", state.LastRunID)
	require.Equal(t, scheduler.TaskStatusRunning, state.Status)
	require.NotZero(t, state.RunStartedAt)
}
