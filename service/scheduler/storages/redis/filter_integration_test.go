// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"

	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
)

func parseFilter(t *testing.T, expr string) filter.Node {
	t.Helper()
	parser, err := filter.NewParser()
	require.NoError(t, err)
	node, err := parser.Parse(t.Context(), expr)
	require.NoError(t, err, expr)
	return node
}

func pageIDs(t *testing.T, s *redisstore.Storage, pg scheduler.Pagination, expr string) []string {
	t.Helper()
	var f filter.Node
	if expr != "" {
		f = parseFilter(t, expr)
	}
	page, err := s.TasksPaginated(t.Context(), pg, f)
	require.NoError(t, err, expr)
	ids := make([]string, 0, len(page))
	for _, st := range page {
		ids = append(ids, st.ID)
	}
	return ids
}

// TestIntegration_RedisTasksPaginatedCursor pins the cursor to "IDs greater
// than AfterID": the walk used to wait for the cursor's own document, so a
// cursor task deleted between pages, or excluded by the filter, produced an
// empty page instead of the tasks after it.
func TestIntegration_RedisTasksPaginatedCursor(t *testing.T) {
	t.Parallel()
	s := newDueIT(t)
	ctx := t.Context()
	for id, status := range map[string]scheduler.TaskStatus{"a": 1, "b": 2, "c": 1, "d": 1} {
		require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: id, Status: status}}))
	}

	require.Equal(t, []string{"c", "d"}, pageIDs(t, s, scheduler.Pagination{Limit: 10, AfterID: "b"}, `status == 1`),
		"a cursor excluded by the filter must not end the walk")

	require.NoError(t, s.DeleteTask(ctx, "a"))
	require.Equal(t, []string{"b", "c", "d"}, pageIDs(t, s, scheduler.Pagination{Limit: 10, AfterID: "a"}, ""),
		"a deleted cursor must not end the walk")
}

// TestIntegration_RedisFilterSemantics checks the filters RediSearch cannot
// evaluate as CEL does, which the storage now evaluates on the client.
func TestIntegration_RedisFilterSemantics(t *testing.T) {
	t.Parallel()
	s := newDueIT(t)
	ctx := t.Context()
	for _, st := range []scheduler.TaskSummary{
		{ID: "a", Status: 1, Description: "Alpha job"},
		{ID: "b", Status: 2, Description: "alpha job", OneShot: true, NextRunAt: 5},
		{ID: "c", Status: 2, Failures: 1},
	} {
		require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: st}))
	}

	pg := scheduler.Pagination{Limit: 10}
	for expr, want := range map[string][]string{
		`id == "A"`:                                               {},
		`id.startsWith("a")`:                                      {"a"},
		`description == "alpha job"`:                              {"b"},
		`description.contains("Alpha")`:                           {"a"},
		`description.endsWith("job")`:                             {"a", "b"},
		`!oneShot && nextRunAt == 0`:                              {"a", "c"},
		`status in []`:                                            {},
		`!(status in [])`:                                         {"a", "b", "c"},
		`!(status > 1) || priority in [1, 2]`:                     {"a"},
		`!(status == 1 && (priority == 1 || failures == 1))`:      {"a", "b", "c"},
		`description != "" && description.substring(0, 1) == "A"`: {"a"},
		`status in [2] && oneShot`:                                {"b"},
		`status == 1 || id == "b"`:                                {"a", "b"},
		`description.size() == 9`:                                 {"a", "b"},
		`status > 1 && description != ""`:                         {"b"},
	} {
		got := pageIDs(t, s, pg, expr)
		require.ElementsMatch(t, want, got, expr)
	}

	_, err := s.TasksPaginated(ctx, pg, parseFilter(t, `status == 1 || secret == 1`))
	require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	_, err = s.HistoryPaginated(ctx, "a", scheduler.HistoryPagination{Pagination: pg}, parseFilter(t, `success || secret == 1`))
	require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
}
