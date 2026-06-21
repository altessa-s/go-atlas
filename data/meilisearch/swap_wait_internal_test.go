// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	msdk "github.com/meilisearch/meilisearch-go"
)

// TestClient_SwapIndexes_BuildsParamsAndReturnsTaskUID verifies each
// SwapPair is translated into an SDK SwapIndexesParams with both UIDs and
// that the task UID is propagated to the caller.
func TestClient_SwapIndexes_BuildsParamsAndReturnsTaskUID(t *testing.T) {
	t.Parallel()

	var captured []*msdk.SwapIndexesParams
	sdk := &fakeSDK{
		swapIndexesFn: func(_ context.Context, params []*msdk.SwapIndexesParams) (*msdk.TaskInfo, error) {
			captured = params
			return &msdk.TaskInfo{TaskUID: 42}, nil
		},
	}
	c := newTestClient(sdk)

	uid, err := c.SwapIndexes(t.Context(),
		SwapPair{Lhs: "workers", Rhs: "workers_new"},
		SwapPair{Lhs: "objects", Rhs: "objects_new"},
	)
	require.NoError(t, err)
	require.Equal(t, int64(42), uid)

	require.Len(t, captured, 2)
	require.Equal(t, []string{"workers", "workers_new"}, captured[0].Indexes)
	require.Equal(t, []string{"objects", "objects_new"}, captured[1].Indexes)
}

// TestClient_SwapIndexes_WrapsSDKError confirms SDK errors round-trip
// through coreerrs so errors.Is still reaches the original.
func TestClient_SwapIndexes_WrapsSDKError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("swap rejected")
	sdk := &fakeSDK{
		swapIndexesFn: func(_ context.Context, _ []*msdk.SwapIndexesParams) (*msdk.TaskInfo, error) {
			return nil, sentinel
		},
	}
	c := newTestClient(sdk)

	_, err := c.SwapIndexes(t.Context(), SwapPair{Lhs: "a", Rhs: "b"})
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
}

// TestClient_SwapIndexes_NoPairs verifies a call with zero pairs fails fast
// with errNoSwapPairs and never reaches the SDK — guarding against a no-op
// task UID the caller might mistakenly await.
func TestClient_SwapIndexes_NoPairs(t *testing.T) {
	t.Parallel()

	called := false
	sdk := &fakeSDK{
		swapIndexesFn: func(_ context.Context, _ []*msdk.SwapIndexesParams) (*msdk.TaskInfo, error) {
			called = true
			return &msdk.TaskInfo{TaskUID: 1}, nil
		},
	}
	c := newTestClient(sdk)

	uid, err := c.SwapIndexes(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, errNoSwapPairs)
	require.Zero(t, uid)
	require.False(t, called, "SDK must not be contacted when no pairs are given")
}

// TestClient_WaitForTask_SucceedsAndPassesArgs verifies the happy path and
// that the caller's taskUID + interval reach the context-aware SDK variant.
func TestClient_WaitForTask_SucceedsAndPassesArgs(t *testing.T) {
	t.Parallel()

	var (
		gotUID      int64
		gotInterval time.Duration
	)
	sdk := &fakeSDK{
		waitForTaskFn: func(_ context.Context, taskUID int64, interval time.Duration) (*msdk.Task, error) {
			gotUID, gotInterval = taskUID, interval
			return &msdk.Task{Status: msdk.TaskStatusSucceeded, TaskUID: taskUID}, nil
		},
	}
	c := newTestClient(sdk)

	require.NoError(t, c.WaitForTask(t.Context(), 7, 50*time.Millisecond))
	require.Equal(t, int64(7), gotUID)
	require.Equal(t, 50*time.Millisecond, gotInterval)
}

// TestClient_WaitForTask_FailsOnNonSuccessStatus verifies a terminal
// non-succeeded task yields an ErrTaskFailed-wrapped error carrying the
// Meilisearch error code for diagnosis.
func TestClient_WaitForTask_FailsOnNonSuccessStatus(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDK{
		waitForTaskFn: func(_ context.Context, taskUID int64, _ time.Duration) (*msdk.Task, error) {
			task := &msdk.Task{Status: msdk.TaskStatusFailed, TaskUID: taskUID}
			task.Error.Code = "index_not_found"
			task.Error.Message = "index `workers_new` not found"
			return task, nil
		},
	}
	c := newTestClient(sdk)

	err := c.WaitForTask(t.Context(), 9, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTaskFailed)
	require.Contains(t, err.Error(), "index_not_found")
}

// TestClient_WaitForTask_WrapsSDKError confirms transport-level errors
// (canceled ctx, network) round-trip through coreerrs.
func TestClient_WaitForTask_WrapsSDKError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("context deadline exceeded")
	sdk := &fakeSDK{
		waitForTaskFn: func(_ context.Context, _ int64, _ time.Duration) (*msdk.Task, error) {
			return nil, sentinel
		},
	}
	c := newTestClient(sdk)

	err := c.WaitForTask(t.Context(), 1, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	require.NotErrorIs(t, err, ErrTaskFailed, "transport errors must not be misclassified as task failures")
}
