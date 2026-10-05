// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// CreateTask verifies insert-if-absent semantics: the first create stores the
// task with revision one, a later create leaves the stored (claimed) task
// untouched, and exactly one of several concurrent creators wins. The supplied
// store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps and counts describe the storage contract.
func CreateTask(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	fresh := func(id string) *scheduler.TaskState {
		return &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
		}, Meta: map[string]string{"k": "v"}, RunAt: 250, Revision: 42}
	}

	created, err := store.CreateTask(ctx, fresh("create"))
	require.NoError(t, err)
	require.True(t, created)
	got, err := store.GetTask(ctx, "create")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(1), got.Revision, "the caller's revision is ignored")
	require.Equal(t, scheduler.TaskStatusActive, got.Status)
	require.Equal(t, int64(300), got.NextRunAt)
	require.Equal(t, map[string]string{"k": "v"}, got.Meta)
	require.Equal(t, int64(250), got.RunAt, "RunAt must survive the round trip")

	claimed, err := store.ClaimRun(ctx, "create", scheduler.RunClaim{NextRunAt: 300, RunAt: 250, StartedAt: 100, RunID: "owner/run"})
	require.NoError(t, err)
	require.True(t, claimed)

	created, err = store.CreateTask(ctx, fresh("create"))
	require.NoError(t, err)
	require.False(t, created, "an existing task must not be replaced")
	got, err = store.GetTask(ctx, "create")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, "owner/run", got.LastRunID)
	require.Equal(t, int64(100), got.RunStartedAt)
	require.Equal(t, int64(2), got.Revision)

	const creators = 8
	var (
		wins atomic.Int32
		wg   sync.WaitGroup
	)
	for range creators {
		wg.Go(func() {
			ok, err := store.CreateTask(ctx, fresh("contended"))
			if err == nil && ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), wins.Load(), "exactly one concurrent creator must win")
}

// RenewRun verifies that only the owner of an unfinished run can extend its
// lease, that a renewal increments the revision, and that the lease survives
// the round trip. The supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func RenewRun(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "renew", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
	}}))

	ok, err := store.RenewRun(ctx, "renew", "owner/run", 500)
	require.NoError(t, err)
	require.False(t, ok, "a task without an unfinished run has no lease to renew")

	claimed, err := store.ClaimRun(ctx, "renew", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: "owner/run"})
	require.NoError(t, err)
	require.True(t, claimed)
	before, err := store.GetTask(ctx, "renew")
	require.NoError(t, err)

	ok, err = store.RenewRun(ctx, "renew", "owner/run", 500)
	require.NoError(t, err)
	require.True(t, ok)
	got, err := store.GetTask(ctx, "renew")
	require.NoError(t, err)
	require.Equal(t, int64(500), got.RunLeaseUntil)
	require.Equal(t, "owner/run", got.RunLeaseID, "a renewal binds the lease to the run")
	require.Equal(t, before.Revision+1, got.Revision)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, int64(100), got.RunStartedAt)

	for _, tc := range []struct{ name, id, runID string }{
		{"other_run", "renew", "other/run"},
		{"empty_run", "renew", ""},
		{"missing_task", "absent", "owner/run"},
	} {
		ok, err = store.RenewRun(ctx, tc.id, tc.runID, 900)
		require.NoError(t, err, tc.name)
		require.False(t, ok, tc.name)
	}

	finished, err := store.FinishRun(ctx, "renew", "owner/run",
		scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 360, Schedule: "@every 1m", Success: true})
	require.NoError(t, err)
	require.True(t, finished)
	ok, err = store.RenewRun(ctx, "renew", "owner/run", 900)
	require.NoError(t, err)
	require.False(t, ok, "a finished run must not be renewed")
	got, err = store.GetTask(ctx, "renew")
	require.NoError(t, err)
	require.Zero(t, got.RunLeaseUntil, "finishing clears the lease")
	require.Empty(t, got.RunLeaseID)

	// A run whose stored lease is unbound or bound to another run — as left by
	// a claim of a release without leases — is bound by its next renewal.
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "unbound", Status: scheduler.TaskStatusRunning, Schedule: "@every 1m"},
		LastRunID:   "legacy/run", RunStartedAt: 100, RunLeaseUntil: 150, RunLeaseID: "earlier/run",
	}))
	ok, err = store.RenewRun(ctx, "unbound", "legacy/run", 700)
	require.NoError(t, err)
	require.True(t, ok)
	got, err = store.GetTask(ctx, "unbound")
	require.NoError(t, err)
	require.Equal(t, int64(700), got.RunLeaseUntil)
	require.Equal(t, "legacy/run", got.RunLeaseID)
}

// ClaimRunRequiresFinishedRun verifies that an active task whose previous run
// is still unfinished — as after a pause and resume during the run — cannot be
// claimed. The supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func ClaimRunRequiresFinishedRun(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary:  scheduler.TaskSummary{ID: "unfinished", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300},
		LastRunID:    "first/run",
		RunStartedAt: 100,
	}))

	claimed, err := store.ClaimRun(ctx, "unfinished", scheduler.RunClaim{NextRunAt: 300, StartedAt: 200, RunID: "second/run"})
	require.NoError(t, err)
	require.False(t, claimed, "a task with an unfinished run must not be claimable")
	claimed, err = store.ClaimRun(ctx, "unfinished", scheduler.RunClaim{NextRunAt: 0, StartedAt: 200, RunID: "second/run"})
	require.NoError(t, err)
	require.False(t, claimed)

	finished, err := store.FinishRun(ctx, "unfinished", "first/run",
		scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 300, Schedule: "@every 1m", Success: true})
	require.NoError(t, err)
	require.True(t, finished)
	claimed, err = store.ClaimRun(ctx, "unfinished", scheduler.RunClaim{NextRunAt: 300, StartedAt: 200, RunID: "second/run"})
	require.NoError(t, err)
	require.True(t, claimed, "the task is claimable once its run has finished")
}

// ClaimRunFencesOccurrence verifies that a claim fences on the registered
// one-shot occurrence (RunAt) as well as on NextRunAt, and that it stores the
// run's first lease. The supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func ClaimRunFencesOccurrence(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "occurrence", Status: scheduler.TaskStatusActive, NextRunAt: 300, OneShot: true},
		RunAt:       250,
	}))

	claimed, err := store.ClaimRun(ctx, "occurrence",
		scheduler.RunClaim{NextRunAt: 300, RunAt: 260, StartedAt: 300, RunID: "a/run", LeaseUntil: 900})
	require.NoError(t, err)
	require.False(t, claimed, "a claim for another registered occurrence must fail")

	claimed, err = store.ClaimRun(ctx, "occurrence",
		scheduler.RunClaim{NextRunAt: 300, RunAt: 250, StartedAt: 300, RunID: "a/run", LeaseUntil: 900})
	require.NoError(t, err)
	require.True(t, claimed)
	got, err := store.GetTask(ctx, "occurrence")
	require.NoError(t, err)
	require.Equal(t, int64(900), got.RunLeaseUntil, "the claim stores the first lease")
	require.Equal(t, "a/run", got.RunLeaseID, "the claim binds the lease to its run")
	require.Equal(t, "a/run", got.LastRunID)
	require.Equal(t, int64(300), got.RunStartedAt)
}

// RunIDRoundTrip verifies that run IDs of any content — instance IDs are free
// text and may contain quotes, unicode or control characters — survive a
// claim, a renewal and the finish unchanged. The supplied store must be
// isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func RunIDRoundTrip(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	runID := "pod \"a\"\a\té /run"
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "run-id", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
	}}))

	claimed, err := store.ClaimRun(ctx, "run-id", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: runID, LeaseUntil: 400})
	require.NoError(t, err)
	require.True(t, claimed)
	got, err := store.GetTask(ctx, "run-id")
	require.NoError(t, err)
	require.Equal(t, runID, got.LastRunID)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)

	renewed, err := store.RenewRun(ctx, "run-id", runID, 500)
	require.NoError(t, err)
	require.True(t, renewed)
	finished, err := store.FinishRun(ctx, "run-id", runID,
		scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 360, Schedule: "@every 1m", Success: true})
	require.NoError(t, err)
	require.True(t, finished)
}

// ClaimRunRejectsInvalidClaim verifies that a claim whose run could never be
// owned — a non-positive start or an empty run ID — fails with
// [scheduler.ErrInvalidRunClaim] and writes nothing. The supplied store must be
// isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func ClaimRunRejectsInvalidClaim(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "invalid", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
	}}))
	before, err := store.GetTask(ctx, "invalid")
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		claim scheduler.RunClaim
	}{
		{"zero_start", scheduler.RunClaim{NextRunAt: 300, StartedAt: 0, RunID: "a/run"}},
		{"negative_start", scheduler.RunClaim{NextRunAt: 300, StartedAt: -1, RunID: "a/run"}},
		{"empty_run_id", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100}},
		{"negative_lease", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: "a/run", LeaseUntil: -1}},
	} {
		claimed, claimErr := store.ClaimRun(ctx, "invalid", tc.claim)
		require.ErrorIs(t, claimErr, scheduler.ErrInvalidRunClaim, tc.name)
		require.False(t, claimed, tc.name)
	}
	got, err := store.GetTask(ctx, "invalid")
	require.NoError(t, err)
	require.Equal(t, before, got, "a rejected claim must not write")
}

// OwnedRunAnyNonZeroStart verifies that ownership requires RunStartedAt != 0,
// not a positive value: a run stored with a negative start (written through
// UpsertTask) is renewable and finishable on every backend alike. The
// supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func OwnedRunAnyNonZeroStart(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "negative", Status: scheduler.TaskStatusRunning, Schedule: "@every 1m"},
		LastRunID:   "owner/run", RunStartedAt: -5,
	}))
	renewed, err := store.RenewRun(ctx, "negative", "owner/run", 500)
	require.NoError(t, err)
	require.True(t, renewed)
	finished, err := store.FinishRun(ctx, "negative", "owner/run",
		scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 360, Schedule: "@every 1m", Success: true})
	require.NoError(t, err)
	require.True(t, finished)
}
