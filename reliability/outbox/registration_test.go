// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	schedtask "github.com/altessa-s/go-horai/task"
)

func TestPartialRegistrationGatesCallbacksAndRetriesMissingTasks(t *testing.T) {
	t.Parallel()
	boom := errors.New("registration failed")
	var failure atomic.Pointer[error]
	failure.Store(&boom)
	var calls int
	reg := &testhelpers.MockTaskRegistrar{ErrFor: func(schedtask.Config) error {
		calls++
		if calls == 2 {
			return *failure.Load()
		}
		return nil
	}}
	ob := New(&recordingStore{}, noopHandler, WithScheduler(reg),
		WithDispatchSchedule("@every 1m"), WithUnlockSchedule("@every 2m"), WithStatsSchedule("@every 3m"))
	require.Zero(t, reg.Count())
	require.ErrorIs(t, ob.RegisterTasks(t.Context()), boom)
	require.Equal(t, 2, reg.Count())
	first, ok := reg.Task(DefaultDispatchTaskID)
	require.True(t, ok)
	require.ErrorIs(t, first.Func(t.Context()), ErrRegistrationIncomplete)
	require.NoError(t, ob.RunUnlockCycle(t.Context()), "the failed task remains available manually")
	failure.Store(new(error))
	require.NoError(t, ob.RegisterTasks(t.Context()))
	require.Equal(t, 4, reg.Count(), "retry must not register dispatch twice")
	require.NoError(t, first.Func(t.Context()))
	require.NoError(t, ob.RegisterTasks(t.Context()))
	require.Equal(t, 4, reg.Count())
}
