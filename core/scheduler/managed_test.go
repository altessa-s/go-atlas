// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/scheduler"
)

var errCycle = errors.New("cycle failed")

func TestManagedTask_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(mt *scheduler.ManagedTask)
		fnErr    error
		wantErr  error
		wantRuns int
	}{
		{
			name:     "zero value executes fn",
			setup:    func(*scheduler.ManagedTask) {},
			wantRuns: 1,
		},
		{
			name:     "propagates fn error",
			setup:    func(*scheduler.ManagedTask) {},
			fnErr:    errCycle,
			wantErr:  errCycle,
			wantRuns: 1,
		},
		{
			name: "blocked after SchedulerFunc",
			setup: func(mt *scheduler.ManagedTask) {
				mt.SchedulerFunc(func(context.Context) error { return nil })
			},
			wantErr:  scheduler.ErrSchedulerManaged,
			wantRuns: 0,
		},
		{
			name:     "blocked after MarkRegistered",
			setup:    func(mt *scheduler.ManagedTask) { mt.MarkRegistered() },
			wantErr:  scheduler.ErrSchedulerManaged,
			wantRuns: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mt scheduler.ManagedTask
			tc.setup(&mt)

			runs := 0
			err := mt.Run(t.Context(), func(context.Context) error {
				runs++
				return tc.fnErr
			})

			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantRuns, runs)
		})
	}
}

func TestManagedTask_SchedulerFunc_ExecutesAndMarks(t *testing.T) {
	t.Parallel()

	var mt scheduler.ManagedTask
	require.False(t, mt.Registered())

	runs := 0
	fn := mt.SchedulerFunc(func(context.Context) error {
		runs++
		return errCycle
	})

	require.True(t, mt.Registered())
	require.ErrorIs(t, fn(t.Context()), errCycle)
	require.Equal(t, 1, runs)

	// The wrapped function keeps working across invocations.
	require.ErrorIs(t, fn(t.Context()), errCycle)
	require.Equal(t, 2, runs)
}

func TestManagedTask_SingleFlight(t *testing.T) {
	t.Parallel()

	var mt scheduler.ManagedTask

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	ctx := t.Context()

	go func() {
		done <- mt.TryRun(ctx, func(context.Context) error {
			close(entered)
			<-release
			return errCycle
		})
	}()

	<-entered

	// While the first execution is in flight, both TryRun and Run collapse
	// to a nil no-op without invoking the function.
	require.NoError(t, mt.TryRun(t.Context(), func(context.Context) error {
		t.Error("overlapping TryRun must not invoke fn")
		return nil
	}))
	require.NoError(t, mt.Run(t.Context(), func(context.Context) error {
		t.Error("overlapping Run must not invoke fn")
		return nil
	}))

	close(release)
	require.ErrorIs(t, <-done, errCycle)

	// The guard resets after completion: the next run executes again.
	runs := 0
	require.NoError(t, mt.Run(t.Context(), func(context.Context) error {
		runs++
		return nil
	}))
	require.Equal(t, 1, runs)
}

func TestManagedTask_TryRun_IgnoresRegistered(t *testing.T) {
	t.Parallel()

	var mt scheduler.ManagedTask
	mt.MarkRegistered()

	runs := 0
	require.NoError(t, mt.TryRun(t.Context(), func(context.Context) error {
		runs++
		return nil
	}))
	require.Equal(t, 1, runs)
}
