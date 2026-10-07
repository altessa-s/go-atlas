// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// historyContracts lists the [scheduler.HistoryStorage] contracts; [RunHistory]
// executes each. [HistoryCleanup] is not among them: a backend may expire
// history on its own and make CleanupHistory a no-op.
var historyContracts = []struct {
	name  string
	check func(*testing.T, scheduler.HistoryStorage)
}{
	{"HistoryOrder", HistoryOrder},
	{"HistoryPagination", HistoryPagination},
	{"HistoryDelete", HistoryDelete},
}

// RunHistory executes every [scheduler.HistoryStorage] contract as a parallel
// subtest named after it, each on a fresh store from newStore, which must
// return an empty store isolated from every other store it returns. A backend
// that implements CleanupHistory runs [HistoryCleanup] as well.
func RunHistory(t *testing.T, newStore func(testing.TB) scheduler.HistoryStorage) {
	t.Helper()
	for _, c := range historyContracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.check(t, newStore(t))
		})
	}
}

// HistoryOrder verifies that History lists one task's entries by StartedAt
// descending, and that task IDs differing only by case or a trailing space
// have separate history. The supplied store must be empty and isolated per
// invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func HistoryOrder(t *testing.T, store scheduler.HistoryStorage) {
	t.Helper()
	ctx := t.Context()
	for _, h := range []scheduler.TaskHistory{
		{ID: "h1", StartedAt: 10}, {ID: "h3", StartedAt: 25}, {ID: "h2", StartedAt: 20}, {ID: "h4", StartedAt: 30},
	} {
		h.TaskID, h.EndedAt = "a", h.StartedAt+1
		require.NoError(t, store.AddHistory(ctx, &h))
	}
	require.Equal(t, []string{"h4", "h3", "h2", "h1"}, seqIDs(t, store.History(ctx, "a"), historyID))

	ids := []string{"task", "task ", "Task"}
	for i, id := range ids {
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: fmt.Sprint("t", i), TaskID: id, StartedAt: 1, EndedAt: 2}))
	}
	for i, id := range ids {
		require.Equal(t, []string{fmt.Sprint("t", i)}, seqIDs(t, store.History(ctx, id), historyID), "task %q", id)
	}
	require.Empty(t, seqIDs(t, store.History(ctx, "missing"), historyID))
}

// HistoryPagination verifies that HistoryPaginated walks one task's history
// by StartedAt then ID descending through the compound cursor, round-trips
// every field, and evaluates filters on every [scheduler.HistoryFilterFields]
// name, treating an empty error as the zero value. The supplied store must be
// empty and isolated per invocation.
//
//nolint:mnd // Fixed timestamps and sizes describe the storage contract.
func HistoryPagination(t *testing.T, store scheduler.HistoryStorage) {
	t.Helper()
	ctx := t.Context()
	entries := []scheduler.TaskHistory{
		{ID: "h1", RunID: "r1", StartedAt: 10, DurationMs: 100, Success: true},
		{ID: "h2", RunID: "r2", StartedAt: 20, DurationMs: 200, Error: "boom"},
		{ID: "h3", RunID: "r3", StartedAt: 20, DurationMs: 300, Success: true},
		{ID: "h4", RunID: "r4", StartedAt: 30, DurationMs: 400, Error: "bang ✓"},
		{ID: "h5", RunID: "r5", StartedAt: 5, DurationMs: 500, Success: true},
	}
	for _, h := range entries {
		h.TaskID, h.EndedAt = "a", h.StartedAt+1
		require.NoError(t, store.AddHistory(ctx, &h))
	}
	require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "other", TaskID: "b", StartedAt: 25, EndedAt: 26}))

	var walked []*scheduler.TaskHistory
	hpg := scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 2}}
	for {
		page, err := store.HistoryPaginated(ctx, "a", hpg, nil)
		require.NoError(t, err)
		require.LessOrEqual(t, len(page), 3)
		n := min(len(page), 2)
		walked = append(walked, page[:n]...)
		if len(page) <= 2 {
			break
		}
		hpg.AfterID, hpg.AfterStartedAt = page[n-1].ID, page[n-1].StartedAt
	}
	require.Equal(t, []string{"h4", "h3", "h2", "h1", "h5"}, sliceIDs(walked, historyID))
	want := entries[3]
	want.TaskID, want.EndedAt = "a", want.StartedAt+1
	require.Equal(t, want, *walked[0], "every field must round-trip")

	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`success && startedAt >= 10`, []string{"h3", "h1"}},
		{`!success`, []string{"h4", "h2"}},
		{`id == "h2" || runId == "r5"`, []string{"h2", "h5"}},
		{`taskId == "a" && endedAt == 21 && durationMs > 250`, []string{"h3"}},
		{`error == ""`, []string{"h3", "h1", "h5"}},
		{`error != ""`, []string{"h4", "h2"}},
		// The ClickHouse translator measures a string in bytes, the others in
		// code points; "bang ✓" is longer than 5 either way.
		{`error.size() < 5`, []string{"h3", "h2", "h1", "h5"}},
		{`error.startsWith("bang")`, []string{"h4"}},
	} {
		page, err := store.HistoryPaginated(ctx, "a", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 100}},
			mustParse(t, tc.expr))
		require.NoError(t, err, tc.expr)
		require.Equal(t, tc.want, sliceIDs(page, historyID), tc.expr)
	}
}

// HistoryDelete verifies, for a store that is a [scheduler.HistoryDeleter],
// that DeleteHistory removes exactly one task's history and that deleting a
// task without history is not an error. It skips any other store. The
// supplied store must be empty and isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func HistoryDelete(t *testing.T, store scheduler.HistoryStorage) {
	t.Helper()
	deleter, ok := store.(scheduler.HistoryDeleter)
	if !ok {
		t.Skip("store is not a scheduler.HistoryDeleter")
	}
	ctx := t.Context()
	for _, id := range []string{"a", "A", "b"} {
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: id + "1", TaskID: id, StartedAt: 1, EndedAt: 2}))
	}
	require.NoError(t, deleter.DeleteHistory(ctx, "a"))
	require.Empty(t, seqIDs(t, store.History(ctx, "a"), historyID))
	require.Equal(t, []string{"A1"}, seqIDs(t, store.History(ctx, "A"), historyID))
	require.Equal(t, []string{"b1"}, seqIDs(t, store.History(ctx, "b"), historyID))
	require.NoError(t, deleter.DeleteHistory(ctx, "missing"))
}

// HistoryCleanup verifies that CleanupHistory removes the entries that ended
// before the retention window and keeps the rest. [RunHistory] leaves it out;
// a backend whose CleanupHistory enforces the retention runs it on its own.
// The supplied store must be empty and isolated per invocation.
//
//nolint:mnd // Fixed ages describe the storage contract.
func HistoryCleanup(t *testing.T, store scheduler.HistoryStorage) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().Unix()
	for i, age := range []int64{7200, 60, 3600} {
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{
			ID: fmt.Sprint("e", i), TaskID: "hist", StartedAt: now - age - 1, EndedAt: now - age, Success: true,
		}))
	}
	require.NoError(t, store.CleanupHistory(ctx, 90*time.Minute))
	require.Equal(t, []string{"e1", "e2"}, seqIDs(t, store.History(ctx, "hist"), historyID))
}
