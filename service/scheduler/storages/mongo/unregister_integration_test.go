// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// TestIntegration_MongoUnregisterPersistedNotRegistered removes, through a
// fresh scheduler, a task an earlier process persisted in MongoDB and this one
// never registered: the task and its history are deleted.
func TestIntegration_MongoUnregisterPersistedNotRegistered(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	ctx := t.Context()

	earlier := scheduler.New(s)
	require.NoError(t, earlier.Register(ctx, corescheduler.TaskConfig{
		ID: "stale", Schedule: "@hourly", Func: func(context.Context) error { return nil },
	}))
	require.NoError(t, s.AddHistory(ctx, &scheduler.TaskHistory{ID: "h1", TaskID: "stale", Success: true}))

	current := scheduler.New(s)
	require.NoError(t, current.Unregister(ctx, "stale"))

	state, err := s.GetTask(ctx, "stale")
	require.NoError(t, err)
	require.Nil(t, state, "the persisted task is deleted")
	for h, err := range s.History(ctx, "stale") {
		require.NoError(t, err)
		require.Failf(t, "history kept", "entry %s survived Unregister", h.ID)
	}

	require.ErrorIs(t, current.Unregister(ctx, "stale"), scheduler.ErrTaskNotRegistered)
}
