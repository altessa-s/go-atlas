// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// TestUnregister_PersistedNotRegistered removes a task an earlier process
// persisted and this process never registered: the task and its history are
// deleted from storage, and a second call reports ErrTaskNotRegistered.
func TestUnregister_PersistedNotRegistered(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	storage := mustNewMemory(t, 100)

	earlier := scheduler.New(storage)
	require.NoError(t, earlier.Register(ctx, corescheduler.TaskConfig{
		ID: "stale", Schedule: "@hourly", Func: func(context.Context) error { return nil },
	}))
	require.NoError(t, storage.AddHistory(ctx, &scheduler.TaskHistory{ID: "h1", TaskID: "stale", Success: true}))

	current := scheduler.New(storage)
	require.NoError(t, current.Unregister(ctx, "stale"))

	state, err := storage.GetTask(ctx, "stale")
	require.NoError(t, err)
	require.Nil(t, state, "the persisted task is deleted")
	for h, err := range storage.History(ctx, "stale") {
		require.NoError(t, err)
		require.Failf(t, "history kept", "entry %s survived Unregister", h.ID)
	}

	require.ErrorIs(t, current.Unregister(ctx, "stale"), scheduler.ErrTaskNotRegistered)
}
