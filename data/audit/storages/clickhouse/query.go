// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/data/audit"
)

// maxQueryConditions is the number of criteria [audit.Query] can express,
// and therefore the largest WHERE clause [buildWhere] can render.
const maxQueryConditions = 11

// timestampParam binds a time as epoch milliseconds rather than letting the
// driver render a time.Time.
//
// The rendered form loses sub-second precision, which is invisible on a
// coarse range filter but fatal for keyset paging: the cursor must compare
// exactly equal to the stored value, and a truncated bound either drops the
// whole page or replays the row it was supposed to resume past. Milliseconds
// are also exactly what DateTime64(3) holds, so nothing is lost or invented
// in the conversion.
const timestampParam = "fromUnixTimestamp64Milli(?)"

// columnsSQL is the backquoted column list in [schemaColumns] order, shared
// by the INSERT and SELECT statements so that both stay positional.
var columnsSQL = columnList()

func columnList() string {
	names := make([]string, len(schemaColumns))
	for i, c := range schemaColumns {
		names[i] = "`" + c.name + "`"
	}

	return strings.Join(names, ", ")
}

// insertStatement returns the INSERT prelude consumed by the batch API. The
// column list is spelled out so that a later ALTER TABLE ... ADD COLUMN
// cannot silently shift positional values.
func insertStatement(table string) string {
	return fmt.Sprintf("INSERT INTO `%s` (%s)", table, columnsSQL)
}

// selectStatement renders the read query and its bind arguments.
func (s *Storage) selectStatement(q *audit.Query, after *audit.Cursor) (string, []any) {
	where, args := buildWhere(q, after)
	limit, limitArgs := limitClause(q)

	stmt := fmt.Sprintf("SELECT %s FROM `%s`%s%s%s%s",
		columnsSQL, s.opts.tableName, s.finalClause(), where, orderClause(q), limit)

	return stmt, append(args, limitArgs...)
}

// countStatement renders the count query and its bind arguments. count() is
// cast so the result scans into an int64 without an unsigned conversion.
func (s *Storage) countStatement(q *audit.Query, after *audit.Cursor) (string, []any) {
	where, args := buildWhere(q, after)

	return fmt.Sprintf("SELECT toInt64(count()) FROM `%s`%s%s", s.opts.tableName, s.finalClause(), where), args
}

// finalClause returns the FINAL modifier when reads must observe the
// deduplicated view of a ReplacingMergeTree table.
func (s *Storage) finalClause() string {
	if s.opts.final {
		return " FINAL"
	}

	return ""
}

// buildWhere renders the WHERE clause for q, with one placeholder per bound
// value. A nil or empty query yields no clause and no arguments.
func buildWhere(q *audit.Query, after *audit.Cursor) (string, []any) {
	if q == nil {
		return "", nil
	}

	conds := make([]string, 0, maxQueryConditions)
	args := make([]any, 0, maxQueryConditions)

	add := func(cond string, arg any) {
		conds = append(conds, cond)
		args = append(args, arg)
	}
	addIf := func(ok bool, cond string, arg any) {
		if ok {
			add(cond, arg)
		}
	}

	if q.StartTime != nil {
		add("`"+colTimestamp+"` >= "+timestampParam, q.StartTime.UnixMilli())
	}
	if q.EndTime != nil {
		add("`"+colTimestamp+"` <= "+timestampParam, q.EndTime.UnixMilli())
	}

	// Tuple comparison against the sort key resumes strictly past the
	// cursor. The direction follows the sort order: descending pages move
	// toward older events, ascending toward newer ones.
	if c := after; c != nil {
		op := "<"
		if q.SortOrder == audit.SortOrderAsc {
			op = ">"
		}

		add(fmt.Sprintf("(`%s`, `%s`) %s (%s, ?)", colTimestamp, colID, op, timestampParam),
			c.Timestamp.UnixMilli())
		args = append(args, c.ID)
	}

	addIf(q.ActorID != "", "`"+colActorID+"` = ?", q.ActorID)
	addIf(q.ActorType != "", "`"+colActorType+"` = ?", q.ActorType)
	addIf(q.ResourceType != "", "`"+colResourceType+"` = ?", q.ResourceType)
	addIf(q.ResourceID != "", "`"+colResourceID+"` = ?", q.ResourceID)
	addIf(q.EventType != "", "`"+colType+"` = ?", string(q.EventType))
	addIf(q.Action != "", "`"+colAction+"` = ?", string(q.Action))
	addIf(q.Status != "", "`"+colResultStatus+"` = ?", string(q.Status))
	addIf(q.RequestID != "", "`"+colRequestID+"` = ?", q.RequestID)
	addIf(q.TraceID != "", "`"+colTraceID+"` = ?", q.TraceID)

	if len(conds) == 0 {
		return "", nil
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// orderClause sorts by event time, newest first unless asked otherwise.
//
// The id breaks ties. Audit events are written in batches, so identical
// timestamps are routine; without a tiebreaker their relative order is
// undefined between queries and cursor paging could skip or repeat a row.
// The id is unique per event, which makes the order total.
func orderClause(q *audit.Query) string {
	if q != nil && q.SortOrder == audit.SortOrderAsc {
		return " ORDER BY `" + colTimestamp + "` ASC, `" + colID + "` ASC"
	}

	return " ORDER BY `" + colTimestamp + "` DESC, `" + colID + "` DESC"
}

// limitClause renders LIMIT. There is no OFFSET to render: paging is
// keyset-based, so the position travels in [audit.Query.Cursor] and lands in
// the WHERE clause, where the sorting key can actually serve it instead of
// reading and discarding every skipped row.
func limitClause(q *audit.Query) (string, []any) {
	if q == nil || q.Limit <= 0 {
		return "", nil
	}

	return " LIMIT ?", []any{q.Limit}
}
