// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// ClaimRun verifies that the claim is an atomic compare-and-swap on status and
// the occurrence fence: exactly one of many concurrent claims wins, and stale
// or paused claims lose. The supplied store must be isolated per invocation.
//
//nolint:mnd // Fixed timestamps and counts describe the storage contract.
func ClaimRun(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	put := func(id string, status scheduler.TaskStatus) {
		require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: status, Schedule: "@every 1m", NextRunAt: 300,
		}}))
	}

	put("claim_race", scheduler.TaskStatusActive)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			ok, err := store.ClaimRun(ctx, "claim_race", 300, 100, fmt.Sprintf("run-%d", i))
			if err == nil && ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), wins.Load(), "exactly one concurrent claim must win")
	got, err := store.GetTask(ctx, "claim_race")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, int64(100), got.RunStartedAt)

	put("claim_stale", scheduler.TaskStatusActive)
	ok, err := store.ClaimRun(ctx, "claim_stale", 299, 100, "run")
	require.NoError(t, err)
	require.False(t, ok, "a stale occurrence fence must lose")

	put("claim_paused", scheduler.TaskStatusPaused)
	ok, err = store.ClaimRun(ctx, "claim_paused", 300, 100, "run")
	require.NoError(t, err)
	require.False(t, ok, "a paused task must not be claimed")

	put("claim_any", scheduler.TaskStatusActive)
	ok, err = store.ClaimRun(ctx, "claim_any", 0, 100, "run")
	require.NoError(t, err)
	require.True(t, ok, "a zero fence claims any occurrence")

	ok, err = store.ClaimRun(ctx, "claim_missing", 0, 100, "run")
	require.NoError(t, err)
	require.False(t, ok)
}

// DueTasks verifies that exactly the active tasks with NextRunAt <= now are
// returned, ordered by ID, and that Tasks returns every task (its order is
// implementation-defined).
// The supplied store must be empty and isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func DueTasks(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	for _, s := range []scheduler.TaskSummary{
		{ID: "due_c", Status: scheduler.TaskStatusActive, NextRunAt: 100},
		{ID: "due_a", Status: scheduler.TaskStatusActive, NextRunAt: 200},
		{ID: "due_b", Status: scheduler.TaskStatusPaused, NextRunAt: 100},
		{ID: "due_d", Status: scheduler.TaskStatusActive, NextRunAt: 201},
		{ID: "due_e", Status: scheduler.TaskStatusRunning, NextRunAt: 50},
	} {
		require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: s}))
	}
	require.Equal(t, []string{"due_a", "due_c"}, taskIDs(t, store.DueTasks(ctx, 200)))
	require.ElementsMatch(t, []string{"due_a", "due_b", "due_c", "due_d", "due_e"}, taskIDs(t, store.Tasks(ctx)))
}

// Identity verifies that task IDs and run-ownership fences compare exactly:
// IDs differing only by case or a trailing space are distinct rows with
// separate history, and a run ID differing only by case does not own a run.
// The supplied store must be empty and isolated per invocation.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func Identity(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	ids := []string{"task", "task ", "Task"}
	for i, id := range ids {
		require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: scheduler.TaskStatusActive, Description: fmt.Sprint(i),
		}, Meta: map[string]string{"emoji": "🚀", "k": "$revision"}}))
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "h" + fmt.Sprint(i), TaskID: id, StartedAt: 1, EndedAt: 2}))
	}
	for i, id := range ids {
		got, err := store.GetTask(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID)
		require.Equal(t, fmt.Sprint(i), got.Description)
		require.Equal(t, map[string]string{"emoji": "🚀", "k": "$revision"}, got.Meta)
		require.Len(t, historyIDs(t, store.History(ctx, id)), 1)
	}
	require.NoError(t, store.DeleteTask(ctx, "task "))
	got, err := store.GetTask(ctx, "task")
	require.NoError(t, err)
	require.NotNil(t, got, "deleting 'task ' must not delete 'task'")
	require.Len(t, historyIDs(t, store.History(ctx, "task")), 1)

	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "owned", Status: scheduler.TaskStatusRunning,
	}, LastRunID: "owner", RunStartedAt: 100}))
	ok, err := store.FinishRun(ctx, "owned", "OWNER", scheduler.RunResult{StartedAt: 100, EndedAt: 110, Success: true})
	require.NoError(t, err)
	require.False(t, ok, "run ownership must be case-sensitive")
}

// Pagination verifies TasksPaginated (ID order, AfterID cursor, filters) and
// HistoryPaginated (StartedAt then ID descending, compound cursor, filters).
// The supplied store must be empty and isolated per invocation.
//
//nolint:mnd // Fixed timestamps and sizes describe the storage contract.
func Pagination(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	ids := []string{"B", "a", "a_1", "a1", "b", "z", "Alpha"}
	for i, id := range ids {
		require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
			ID: id, Status: scheduler.TaskStatusActive, Priority: scheduler.TaskPriority(i % 3), OneShot: i%2 == 0,
			Description: map[bool]string{true: "Alpha job", false: "alpha job"}[i%2 == 0],
		}}))
	}
	want := slices.Sorted(slices.Values(ids))

	var walked []string
	pg := scheduler.Pagination{Limit: 2}
	for {
		page, err := store.TasksPaginated(ctx, pg, nil)
		require.NoError(t, err)
		require.LessOrEqual(t, len(page), 3)
		n := min(len(page), 2)
		for _, s := range page[:n] {
			walked = append(walked, s.ID)
		}
		if len(page) <= 2 {
			break
		}
		pg.AfterID = page[n-1].ID
	}
	require.Equal(t, want, walked, "pages must walk every task once in byte-wise ID order")

	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`status == 1 && priority > 0`, []string{"a", "a_1", "b", "z"}},
		{`oneShot`, []string{"Alpha", "B", "a_1", "b"}},
		{`id.startsWith("a")`, []string{"a", "a1", "a_1"}},
		{`description == "Alpha job"`, []string{"Alpha", "B", "a_1", "b"}},
		{`description.matches("^Alpha")`, []string{"Alpha", "B", "a_1", "b"}},
		{`description.endsWith("job") && description.size() == 9`, want},
	} {
		page, err := store.TasksPaginated(ctx, scheduler.Pagination{Limit: 100}, mustParse(t, tc.expr))
		require.NoError(t, err, tc.expr)
		require.Equal(t, slices.Sorted(slices.Values(tc.want)), stateIDs(page), tc.expr)
	}

	// History: ties on StartedAt are broken by ID descending.
	for _, h := range []scheduler.TaskHistory{
		{ID: "h1", StartedAt: 10, Success: true}, {ID: "h2", StartedAt: 20}, {ID: "h3", StartedAt: 20, Success: true},
		{ID: "h4", StartedAt: 30, Error: "boom"}, {ID: "h5", StartedAt: 5, Success: true},
	} {
		h.TaskID, h.EndedAt = "a", h.StartedAt+1
		require.NoError(t, store.AddHistory(ctx, &h))
	}
	require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "other", TaskID: "b", StartedAt: 25}))

	var hist []string
	hpg := scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 2}}
	for {
		page, err := store.HistoryPaginated(ctx, "a", hpg, nil)
		require.NoError(t, err)
		n := min(len(page), 2)
		for _, h := range page[:n] {
			hist = append(hist, h.ID)
		}
		if len(page) <= 2 {
			break
		}
		hpg.AfterID, hpg.AfterStartedAt = page[n-1].ID, page[n-1].StartedAt
	}
	require.Equal(t, []string{"h4", "h3", "h2", "h1", "h5"}, hist)

	page, err := store.HistoryPaginated(ctx, "a", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 100}},
		mustParse(t, `success && startedAt >= 10`))
	require.NoError(t, err)
	require.Equal(t, []string{"h3", "h1"}, historyPageIDs(page))
}

// History verifies History ordering, CleanupHistory retention and that
// DeleteTask removes a task's history. The supplied store must be empty and
// isolated per invocation.
//
//nolint:mnd // Fixed ages describe the storage contract.
func History(t *testing.T, store scheduler.Storage) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().Unix()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "hist", Status: scheduler.TaskStatusActive}}))
	for i, age := range []int64{7200, 60, 3600} {
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{
			ID: fmt.Sprint("e", i), TaskID: "hist", StartedAt: now - age - 1, EndedAt: now - age, Success: true,
		}))
	}
	require.Equal(t, []string{"e1", "e2", "e0"}, historyIDs(t, store.History(ctx, "hist")))

	require.NoError(t, store.CleanupHistory(ctx, 90*time.Minute))
	require.Equal(t, []string{"e1", "e2"}, historyIDs(t, store.History(ctx, "hist")))

	require.NoError(t, store.DeleteTask(ctx, "hist"))
	got, err := store.GetTask(ctx, "hist")
	require.NoError(t, err)
	require.Nil(t, got)
	require.Empty(t, historyIDs(t, store.History(ctx, "hist")))
	require.NoError(t, store.DeleteTask(ctx, "hist"), "deleting a missing task is not an error")
}

func mustParse(t *testing.T, expr string) filter.Node {
	t.Helper()
	parser, err := filter.NewParser()
	require.NoError(t, err)
	node, err := parser.Parse(context.Background(), expr)
	require.NoError(t, err, expr)
	return node
}

func taskIDs(t *testing.T, seq func(func(*scheduler.TaskState, error) bool)) []string {
	t.Helper()
	var ids []string
	for s, err := range seq {
		require.NoError(t, err)
		ids = append(ids, s.ID)
	}
	return ids
}

func historyIDs(t *testing.T, seq func(func(*scheduler.TaskHistory, error) bool)) []string {
	t.Helper()
	var ids []string
	for h, err := range seq {
		require.NoError(t, err)
		ids = append(ids, h.ID)
	}
	return ids
}

func stateIDs(states []*scheduler.TaskState) []string {
	ids := make([]string, 0, len(states))
	for _, s := range states {
		ids = append(ids, s.ID)
	}
	return ids
}

func historyPageIDs(hist []*scheduler.TaskHistory) []string {
	ids := make([]string, 0, len(hist))
	for _, h := range hist {
		ids = append(ids, h.ID)
	}
	return ids
}
