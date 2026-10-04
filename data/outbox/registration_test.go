// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

type partialRegistrar struct {
	testhelpers.MockTaskRegistrar
	failure error
}

func (r *partialRegistrar) Register(ctx context.Context, task corescheduler.TaskConfig) error {
	err := r.MockTaskRegistrar.Register(ctx, task)
	if r.Count() == 2 {
		return r.failure
	}
	return err
}

func TestPartialRegistrationGatesCallbacksAndRetriesMissingTasks(t *testing.T) {
	t.Parallel()
	boom := errors.New("registration failed")
	reg := &partialRegistrar{failure: boom}
	ob := New(&recordingStore{}, noopHandler, WithScheduler(reg),
		WithDispatchSchedule("@every 1m"), WithUnlockSchedule("@every 2m"), WithStatsSchedule("@every 3m"))
	require.Zero(t, reg.Count())
	require.ErrorIs(t, ob.RegisterTasks(t.Context()), boom)
	require.Equal(t, 2, reg.Count())
	first, ok := reg.Task(DefaultDispatchTaskID)
	require.True(t, ok)
	require.ErrorIs(t, first.Func(t.Context()), ErrRegistrationIncomplete)
	require.NoError(t, ob.RunUnlockCycle(t.Context()), "the failed task remains available manually")
	reg.failure = nil
	require.NoError(t, ob.RegisterTasks(t.Context()))
	require.Equal(t, 4, reg.Count(), "retry must not register dispatch twice")
	require.NoError(t, first.Func(t.Context()))
	require.NoError(t, ob.RegisterTasks(t.Context()))
	require.Equal(t, 4, reg.Count())
}
