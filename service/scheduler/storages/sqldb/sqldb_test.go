// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
)

var errBoom = errors.New("boom")

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New(db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	for _, name := range []string{"t; DROP TABLE x", "a.b.c", "1tasks", `"quoted"`, "tab le", strings.Repeat("t", 64), "s." + strings.Repeat("t", 64)} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTasksTable(name))
		require.ErrorIs(t, err, sqldb.ErrInvalidTableName, name)
	}
	_, err = sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTasksTable("app.sched_tasks"), sqldb.WithHistoryTable("sched_history"))
	require.NoError(t, err)
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTasksTable(strings.Repeat("t", 63)))
	require.NoError(t, err, "63 characters is the limit, not over it")
}

func TestEnsureSchemaPicksEngineCollation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		version string
		want    string
		err     error
	}{
		{"8.4.2", "utf8mb4_0900_bin", nil},
		{"8.0.17-log", "utf8mb4_0900_bin", nil},
		{"8.0.16", "", sqldb.ErrUnsupportedVersion},
		{"11.4.2-MariaDB-ubu2404", "utf8mb4_nopad_bin", nil},
		{"10.6.0-MariaDB", "utf8mb4_nopad_bin", nil},
		{"10.5.9-MariaDB", "", sqldb.ErrUnsupportedVersion},
	} {
		t.Run(tc.version, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				if q == "SELECT VERSION()" {
					return testhelpers.FakeSQLReply{Columns: []string{"v"}, Rows: [][]driver.Value{{tc.version}}}
				}
				return testhelpers.FakeSQLReply{}
			})
			store, err := sqldb.New(db, sqldb.DialectMySQL)
			require.NoError(t, err)

			err = store.EnsureSchema(t.Context())
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			calls := fake.Calls()
			require.Len(t, calls, 3) // version probe + two tables
			for _, c := range calls[1:] {
				require.Contains(t, c.Query, "COLLATE "+tc.want)
				require.NotContains(t, c.Query, "utf8mb4_bin ", "PAD SPACE collations must never be used")
			}
		})
	}
}

func TestEnsureSchemaIndexNamesFitIdentifierLimit(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	long := strings.Repeat("t", 62)
	store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTasksTable(long+"a"), sqldb.WithHistoryTable(long+"b"))
	require.NoError(t, err)
	require.NoError(t, store.EnsureSchema(t.Context()))

	names := map[string]bool{}
	for _, c := range fake.Calls() {
		for _, m := range regexp.MustCompile(`INDEX IF NOT EXISTS "([^"]+)"`).FindAllStringSubmatch(c.Query, -1) {
			require.LessOrEqual(t, len(m[1]), 63, m[1])
			names[m[1]] = true
		}
	}
	require.Len(t, names, 3, "truncated index names must stay distinct")
}

func TestPlaceholderStyle(t *testing.T) {
	t.Parallel()
	for _, d := range []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			store, err := sqldb.New(db, d)
			require.NoError(t, err)
			ctx := t.Context()
			require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "a"}}))
			_, err = store.ClaimRun(ctx, "a", 1, 2, "r")
			require.NoError(t, err)
			_, err = store.FinishRun(ctx, "a", "r", scheduler.RunResult{})
			require.NoError(t, err)
			_, err = store.ReplaceTaskIf(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "a"}}, scheduler.TaskFence{})
			require.NoError(t, err)

			for _, c := range fake.Calls() {
				if d == sqldb.DialectPostgres {
					require.NotContains(t, c.Query, "?", c.Query)
					require.Len(t, regexp.MustCompile(`\$\d+`).FindAllString(c.Query, -1), len(c.Args), c.Query)
				} else {
					require.NotRegexp(t, `\$\d`, c.Query)
					require.Equal(t, strings.Count(c.Query, "?"), len(c.Args), c.Query)
				}
			}
		})
	}
}

func TestFilteredPaginationNumbersPlaceholdersAfterTheFilter(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Columns: taskCols()} })
	store, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)
	parser, err := filter.NewParser()
	require.NoError(t, err)
	node, err := parser.Parse(t.Context(), `status == 1 && priority in [1, 2]`)
	require.NoError(t, err)

	_, err = store.TasksPaginated(t.Context(), scheduler.Pagination{AfterID: "k", Limit: 5}, node)
	require.NoError(t, err)
	c := fake.Calls()[0]
	require.Contains(t, c.Query, "id > $4")
	require.Contains(t, c.Query, "LIMIT $5")
	require.Equal(t, []any{int64(1), int64(1), int64(2), "k", int64(6)}, c.Args)

	_, err = store.HistoryPaginated(t.Context(), "task", scheduler.HistoryPagination{
		Pagination: scheduler.Pagination{AfterID: "h", Limit: 5}, AfterStartedAt: 9,
	}, nil)
	require.NoError(t, err)
	c = fake.Calls()[1]
	require.Contains(t, c.Query, "task_id = $1 AND (started_at < $2 OR (started_at = $3 AND id < $4))")
	require.Contains(t, c.Query, "LIMIT $5")
}

func TestResultMapping(t *testing.T) {
	t.Parallel()
	var affected int64
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "SELECT") {
			return testhelpers.FakeSQLReply{Columns: taskCols()} // no rows
		}
		return testhelpers.FakeSQLReply{Affected: affected}
	})
	store, err := sqldb.New(db, sqldb.DialectMySQL)
	require.NoError(t, err)
	ctx := t.Context()

	got, err := store.GetTask(ctx, "missing")
	require.NoError(t, err)
	require.Nil(t, got, "a missing task is (nil, nil)")

	for _, n := range []int64{0, 1} {
		affected = n
		ok, err := store.ClaimRun(ctx, "a", 0, 1, "r")
		require.NoError(t, err)
		require.Equal(t, n == 1, ok)
	}

	before := len(fake.Calls())
	ok, err := store.FinishRun(ctx, "a", "", scheduler.RunResult{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Len(t, fake.Calls(), before, "an empty run ID must not reach the database")
}

func TestWriteErrorsAreWrapped(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: errBoom} })
	store, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)
	_, err = store.ClaimRun(t.Context(), "a", 0, 1, "r")
	require.ErrorIs(t, err, errBoom)
	for _, err := range store.Tasks(t.Context()) {
		require.ErrorIs(t, err, errBoom)
	}
}

func TestIteratorClosesRowsOnBreak(t *testing.T) {
	t.Parallel()
	row := []driver.Value{"a", "", int64(1), int64(0), "", int64(0), int64(0), "", int64(0), int64(0),
		false, false, false, false, `{"k":"v"}`, int64(0), int64(0), int64(1)}
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: taskCols(), Rows: [][]driver.Value{row, row, row}}
	})
	store, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)

	for state, err := range store.Tasks(t.Context()) {
		require.NoError(t, err)
		require.Equal(t, map[string]string{"k": "v"}, state.Meta)
		break
	}
	require.Equal(t, 1, fake.RowsClosed())
}

func taskCols() []string {
	return strings.Split("id,description,status,priority,schedule,last_run_at,next_run_at,last_run_id,run_started_at,failures,"+
		"skip_next_run,disable_history,unmanaged,one_shot,meta,created_at,updated_at,revision", ",")
}

func TestOverlongValuesAreRejected(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	store, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)
	ctx := t.Context()
	limit := strings.Repeat("a", sqldb.MaxIDLength)

	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: limit}}))
	writes := len(fake.Calls())

	// One trailing space past the limit: PostgreSQL would trim it and hit the
	// row above, so it must never reach the database.
	over := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: limit + " "}}
	require.ErrorIs(t, store.UpsertTask(ctx, over), sqldb.ErrValueTooLong)
	_, err = store.ReplaceTaskIf(ctx, over, scheduler.FenceOf(over))
	require.ErrorIs(t, err, sqldb.ErrValueTooLong)
	require.ErrorIs(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "h", TaskID: limit + " "}), sqldb.ErrValueTooLong)
	require.ErrorIs(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: limit + "x", TaskID: "t"}), sqldb.ErrValueTooLong)
	_, err = store.ClaimRun(ctx, "t", 0, 1, limit+"x")
	require.ErrorIs(t, err, sqldb.ErrValueTooLong)
	long := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "t", Schedule: strings.Repeat("*", sqldb.MaxScheduleLength+1)}}
	require.ErrorIs(t, store.UpsertTask(ctx, long), sqldb.ErrValueTooLong)
	require.Len(t, fake.Calls(), writes, "rejected writes must not reach the database")

	// The limit counts characters, not bytes.
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: strings.Repeat("é", sqldb.MaxIDLength)}}))
}

func TestEveryFilterFieldMapsToAColumn(t *testing.T) {
	t.Parallel()
	columns := map[string]string{
		"lastRunAt": "last_run_at", "nextRunAt": "next_run_at", "skipNextRun": "skip_next_run",
		"disableHistory": "disable_history", "oneShot": "one_shot", "taskId": "task_id", "runId": "run_id",
		"startedAt": "started_at", "endedAt": "ended_at", "durationMs": "duration_ms",
	}
	column := func(field string) string {
		if c, ok := columns[field]; ok {
			return c
		}
		return field
	}
	parser, err := filter.NewParser()
	require.NoError(t, err)
	for _, d := range []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Columns: taskCols()} })
			store, err := sqldb.New(db, d)
			require.NoError(t, err)
			for _, field := range scheduler.TaskFilterFields {
				node, err := parser.Parse(t.Context(), field+" != null")
				require.NoError(t, err)
				_, err = store.TasksPaginated(t.Context(), scheduler.Pagination{Limit: 1}, node)
				require.NoError(t, err, field)
				calls := fake.Calls()
				require.Regexp(t, `[`+"`"+`"]`+column(field)+`[`+"`"+`"] IS NOT NULL`, calls[len(calls)-1].Query, field)
			}
			for _, field := range scheduler.HistoryFilterFields {
				node, err := parser.Parse(t.Context(), field+" != null")
				require.NoError(t, err)
				_, err = store.HistoryPaginated(t.Context(), "t", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 1}}, node)
				require.NoError(t, err, field)
				calls := fake.Calls()
				require.Regexp(t, `[`+"`"+`"]`+column(field)+`[`+"`"+`"] IS NOT NULL`, calls[len(calls)-1].Query, field)
			}
		})
	}
}

func TestMySQLFinishRunAssignmentOrder(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	store, err := sqldb.New(db, sqldb.DialectMySQL)
	require.NoError(t, err)
	_, err = store.FinishRun(t.Context(), "a", "r", scheduler.RunResult{})
	require.NoError(t, err)

	// MySQL evaluates SET left to right with already-updated values, so no
	// assignment may read a column assigned before it.
	q := fake.Calls()[0].Query
	set := q[strings.Index(q, " SET ")+5 : strings.Index(q, " WHERE ")]
	starts := regexp.MustCompile(`(?:^|, )(\w+) = `).FindAllStringSubmatchIndex(set, -1)
	var assigned []string
	for k, m := range starts {
		end := len(set)
		if k+1 < len(starts) {
			end = starts[k+1][0]
		}
		col, expr := set[m[2]:m[3]], set[m[1]:end]
		for _, prev := range assigned {
			require.NotContains(t, expr, prev, "%s reads %s, which is assigned earlier", col, prev)
		}
		assigned = append(assigned, col)
	}
	require.Contains(t, assigned, "status")
	require.Contains(t, assigned, "next_run_at")
}
