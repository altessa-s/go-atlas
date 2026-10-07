// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
)

func TestInsertStatement(t *testing.T) {
	t.Parallel()

	stmt := insertStatement("audit_events")

	require.True(t, strings.HasPrefix(stmt, "INSERT INTO `audit_events` (`timestamp`, `id`, "), stmt)
	require.True(t, strings.HasSuffix(stmt, "`metadata`)"), stmt)
	require.Equal(t, len(schemaColumns)-1, strings.Count(stmt, ", "))

	for _, c := range schemaColumns {
		require.Contains(t, stmt, "`"+c.name+"`", "column %s", c.name)
	}
}

func TestBuildWhereEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query *audit.Query
	}{
		{name: "nil query"},
		{name: "no criteria", query: &audit.Query{}},
		{name: "only paging", query: &audit.Query{Limit: 10, SortOrder: audit.SortOrderAsc}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			where, args := buildWhere(tt.query, nil)

			require.Empty(t, where)
			require.Empty(t, args)
		})
	}
}

// One case covers every criterion at once: it pins both the rendered order
// and the fact that typed enums are bound as plain strings.
func TestBuildWhereAllCriteria(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)

	where, args := buildWhere(&audit.Query{
		StartTime:    &start,
		EndTime:      &end,
		ActorID:      "user-42",
		ActorType:    "user",
		ResourceType: "invoice",
		ResourceID:   "inv-7",
		EventType:    audit.EventTypeAPIRequest,
		Action:       audit.ActionUpdate,
		Status:       audit.ResultStatusDenied,
		RequestID:    "req-1",
		TraceID:      "trace-1",
	}, nil)

	require.Equal(t, " WHERE `timestamp` >= fromUnixTimestamp64Milli(?) AND `timestamp` <= fromUnixTimestamp64Milli(?)"+
		" AND `actor_id` = ? AND `actor_type` = ?"+
		" AND `resource_type` = ? AND `resource_id` = ?"+
		" AND `type` = ? AND `action` = ? AND `result_status` = ?"+
		" AND `request_id` = ? AND `trace_id` = ?", where)

	require.Equal(t, []any{
		start.UnixMilli(), end.UnixMilli(),
		"user-42", "user",
		"invoice", "inv-7",
		"api.request", "update", "denied",
		"req-1", "trace-1",
	}, args)
}

func TestBuildWhereSingleBound(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	where, args := buildWhere(&audit.Query{StartTime: &ts}, nil)
	require.Equal(t, " WHERE `timestamp` >= fromUnixTimestamp64Milli(?)", where)
	require.Equal(t, []any{ts.UnixMilli()}, args)

	where, args = buildWhere(&audit.Query{EndTime: &ts}, nil)
	require.Equal(t, " WHERE `timestamp` <= fromUnixTimestamp64Milli(?)", where)
	require.Equal(t, []any{ts.UnixMilli()}, args)
}

// The cursor compares the whole sort key as a tuple, so a page resumes
// strictly past it even when several events share a timestamp. The direction
// follows the sort order.
func TestBuildWhereCursor(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	cursor := &audit.Cursor{Timestamp: ts, ID: "evt-7"}

	where, args := buildWhere(&audit.Query{}, cursor)
	require.Equal(t, " WHERE (`timestamp`, `id`) < (fromUnixTimestamp64Milli(?), ?)", where)
	require.Equal(t, []any{ts.UnixMilli(), "evt-7"}, args)

	where, args = buildWhere(&audit.Query{SortOrder: audit.SortOrderAsc}, cursor)
	require.Equal(t, " WHERE (`timestamp`, `id`) > (fromUnixTimestamp64Milli(?), ?)", where)
	require.Equal(t, []any{ts.UnixMilli(), "evt-7"}, args)
}

// The cursor is one criterion among others and must combine with them.
func TestBuildWhereCursorWithCriteria(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	where, args := buildWhere(&audit.Query{
		StartTime: &ts,
		ActorID:   "user-42",
	}, &audit.Cursor{Timestamp: ts, ID: "evt-7"})

	require.Equal(t, " WHERE `timestamp` >= fromUnixTimestamp64Milli(?) AND (`timestamp`, `id`) < (fromUnixTimestamp64Milli(?), ?) AND `actor_id` = ?", where)
	require.Equal(t, []any{ts.UnixMilli(), ts.UnixMilli(), "evt-7", "user-42"}, args)
}

// The id tiebreaker is what makes keyset paging stable across queries when
// several events share a timestamp.
func TestOrderClause(t *testing.T) {
	t.Parallel()

	require.Equal(t, " ORDER BY `timestamp` DESC, `id` DESC", orderClause(nil))
	require.Equal(t, " ORDER BY `timestamp` DESC, `id` DESC", orderClause(&audit.Query{}))
	require.Equal(t, " ORDER BY `timestamp` DESC, `id` DESC",
		orderClause(&audit.Query{SortOrder: audit.SortOrderDesc}))
	require.Equal(t, " ORDER BY `timestamp` ASC, `id` ASC",
		orderClause(&audit.Query{SortOrder: audit.SortOrderAsc}))
}

func TestLimitClause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    *audit.Query
		wantSQL  string
		wantArgs []any
	}{
		{name: "nil query"},
		{name: "unbounded", query: &audit.Query{}},
		{name: "limit", query: &audit.Query{Limit: 10}, wantSQL: " LIMIT ?", wantArgs: []any{10}},
		{name: "negative limit ignored", query: &audit.Query{Limit: -1}},
		{
			// The cursor never reaches this clause: it belongs in WHERE,
			// where the sorting key can serve it.
			name:  "cursor does not page here",
			query: &audit.Query{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sql, args := limitClause(tt.query)

			require.Equal(t, tt.wantSQL, sql)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestSelectStatement(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	query := &audit.Query{StartTime: &ts, ActorID: "user-42", Limit: 10}

	s := &Storage{opts: newOptions(WithTableName("events"))}
	stmt, args := s.selectStatement(query, nil)

	require.Equal(t, "SELECT "+columnsSQL+" FROM `events`"+
		" WHERE `timestamp` >= fromUnixTimestamp64Milli(?) AND `actor_id` = ?"+
		" ORDER BY `timestamp` DESC, `id` DESC LIMIT ?", stmt)
	require.Equal(t, []any{ts.UnixMilli(), "user-42", 10}, args)
}

func TestSelectStatementFinal(t *testing.T) {
	t.Parallel()

	s := &Storage{opts: newOptions(WithFinal())}
	stmt, args := s.selectStatement(nil, nil)

	require.Equal(t, "SELECT "+columnsSQL+" FROM `audit_events` FINAL ORDER BY `timestamp` DESC, `id` DESC", stmt)
	require.Empty(t, args)
}

func TestCountStatement(t *testing.T) {
	t.Parallel()

	s := &Storage{opts: newOptions()}
	stmt, args := s.countStatement(&audit.Query{ActorID: "user-42", Limit: 10}, nil)

	require.Equal(t, "SELECT toInt64(count()) FROM `audit_events` WHERE `actor_id` = ?", stmt)
	require.Equal(t, []any{"user-42"}, args)
}
