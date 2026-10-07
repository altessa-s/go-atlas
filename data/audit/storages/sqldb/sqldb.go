// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// columnsPerRow is the number of values one event binds: the ID, the
// timestamp, the filter columns and the payload.
var columnsPerRow = 3 + len(filterColumns)

// maxParams is PostgreSQL's limit of bind parameters per statement; the rows
// of one INSERT are capped so that it holds whatever WithMaxBatchRows says.
const maxParams = 65535

var _ audit.Storage = (*Storage)(nil)

// Storage implements [audit.Storage] on a SQL table through the standard
// database/sql package. Each event is one row: the queryable fields as
// columns, the whole event as a JSON payload. Methods are safe for concurrent
// use.
type Storage struct {
	db        *sql.DB
	dialect   dialect
	table     string // quoted
	tableName string // unqualified, for index names and the DDL lock
	opts      *options
}

// New creates a [Storage] over db for the given dialect. It performs no I/O;
// call [Storage.EnsureSchema] once at startup to create the table, or apply
// the equivalent DDL through a migration tool.
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	storage, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := storage.EnsureSchema(ctx); err != nil {
//		return err
//	}
func New(db *sql.DB, d Dialect, opts ...Option) (*Storage, error) {
	if db == nil {
		return nil, errors.New("sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)
	o.maxBatchRows = min(o.maxBatchRows, maxParams/columnsPerRow)
	s := &Storage{db: db, dialect: dl, tableName: sqldialect.Unqualified(o.tableName), opts: o}
	if s.table, err = dl.Table(o.tableName); err != nil {
		return nil, err
	}
	return s, nil
}

// Store persists one event. Storing an event whose ID is already stored is a
// no-op, so a retried delivery cannot fail on its own earlier success.
func (s *Storage) Store(ctx context.Context, event *audit.Event) error {
	return s.StoreBatch(ctx, []*audit.Event{event})
}

// StoreBatch persists the events atomically: in one transaction of INSERTs of
// at most the configured rows each. Events whose ID is already stored —
// earlier, or earlier in the same batch — are skipped, so a batch retried
// after a lost commit acknowledgment succeeds instead of failing forever.
func (s *Storage) StoreBatch(ctx context.Context, events []*audit.Event) error {
	if len(events) == 0 {
		return nil
	}
	args := make([]any, 0, len(events)*columnsPerRow)
	for _, e := range events {
		var err error
		if args, err = s.appendRow(args, e); err != nil {
			return err
		}
	}

	if len(events) <= s.opts.maxBatchRows {
		_, err := s.db.ExecContext(ctx, s.insert(len(events)), args...)
		return coreerrs.WrapOperation(err, "store audit events")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreerrs.WrapOperation(err, "store audit events")
	}
	defer func() { _ = tx.Rollback() }()
	full := s.insert(s.opts.maxBatchRows) // every chunk but possibly the last
	for start := 0; start < len(events); start += s.opts.maxBatchRows {
		n := min(s.opts.maxBatchRows, len(events)-start)
		query := full
		if n < s.opts.maxBatchRows {
			query = s.insert(n)
		}
		chunk := args[start*columnsPerRow : (start+n)*columnsPerRow]
		if _, err := tx.ExecContext(ctx, query, chunk...); err != nil {
			return coreerrs.WrapOperation(err, "store audit events")
		}
	}
	return coreerrs.WrapOperation(tx.Commit(), "store audit events")
}

// insert renders an idempotent INSERT of n rows: ON CONFLICT DO NOTHING on
// PostgreSQL, a no-op ON DUPLICATE KEY UPDATE on MySQL (unlike INSERT IGNORE,
// it lets every other error through).
func (s *Storage) insert(n int) string {
	row := "(?" + strings.Repeat(", ?", columnsPerRow-1) + ")"
	var b strings.Builder
	b.WriteString("INSERT INTO " + s.table + " (id, ts_ms, " + strings.Join(filterColumns, ", ") + ", payload) VALUES ")
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(row)
	}
	if s.dialect.name == DialectPostgres {
		b.WriteString(" ON CONFLICT (id) DO NOTHING")
		return s.dialect.Bind(b.String(), 1)
	}
	b.WriteString(" ON DUPLICATE KEY UPDATE id = id")
	return b.String()
}

// appendRow appends the bound values of one event, in insert column order, to
// args.
func (s *Storage) appendRow(args []any, e *audit.Event) ([]any, error) {
	if n := utf8.RuneCountInString(e.ID); n > MaxIDLength {
		return nil, fmt.Errorf("%w: event ID of %d characters, limit %d", ErrValueTooLong, n, MaxIDLength)
	}
	for _, v := range []string{string(e.Type), string(e.Action), e.Actor.ID, string(e.Actor.Type), e.Resource.Type, e.Resource.ID,
		string(e.Result.Status), e.Context.RequestID, e.Context.TraceID} {
		if len(v) > MaxFilterBytes {
			return nil, fmt.Errorf("%w: event %q has a queryable field of %d bytes, limit %d", ErrValueTooLong, e.ID, len(v),
				MaxFilterBytes)
		}
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "encode audit event")
	}
	t := s.dialect.textArg
	return append(args,
		t(e.ID), e.Timestamp.UnixMilli(),
		t(string(e.Type)), t(string(e.Action)), t(e.Actor.ID), t(string(e.Actor.Type)), t(e.Resource.Type), t(e.Resource.ID),
		t(string(e.Result.Status)), t(e.Context.RequestID), t(e.Context.TraceID),
		s.dialect.jsonArg(payload),
	), nil
}

// Query returns an iterator over the events matching the query, ordered by
// (timestamp millisecond, ID) — the ID byte-wise — newest first unless
// ascending order is requested, and starting after query.Cursor. Stopping the
// iteration closes the result set.
func (s *Storage) Query(ctx context.Context, query *audit.Query) iter.Seq2[*audit.Event, error] {
	return func(yield func(*audit.Event, error) bool) {
		where, args := s.where(query)
		dir, op := "DESC", "<"
		if query.SortOrder == audit.SortOrderAsc {
			dir, op = "ASC", ">"
		}
		if c := query.Cursor; c != nil {
			ms := c.Timestamp.UnixMilli()
			where = append(where, "(ts_ms "+op+" ? OR (ts_ms = ? AND id "+op+" ?))")
			args = append(args, ms, ms, s.dialect.textArg(c.ID))
		}
		q := "SELECT payload FROM " + s.table + whereClause(where) + " ORDER BY ts_ms " + dir + ", id " + dir
		if query.Limit > 0 {
			q += " LIMIT ?"
			args = append(args, query.Limit)
		}

		rows, err := s.db.QueryContext(ctx, s.dialect.Bind(q, 1), args...)
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "query audit events"))
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			// The decoded event owns its data, so the driver's buffer is
			// read in place rather than copied.
			var payload sql.RawBytes
			if err := rows.Scan(&payload); err != nil {
				yield(nil, coreerrs.WrapOperation(err, "scan audit event"))
				return
			}
			var e audit.Event
			if err := json.Unmarshal(payload, &e); err != nil {
				yield(nil, coreerrs.WrapOperation(err, "decode audit event"))
				return
			}
			if !yield(&e, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, coreerrs.WrapOperation(err, "iterate audit events"))
		}
	}
}

// Count returns the number of events matching the query filter. The page
// position, size and order are ignored.
func (s *Storage) Count(ctx context.Context, query *audit.Query) (int64, error) {
	where, args := s.where(query)
	var n int64
	err := s.db.QueryRowContext(ctx, s.dialect.Bind("SELECT COUNT(*) FROM "+s.table+whereClause(where), 1), args...).Scan(&n)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "count audit events")
	}
	return n, nil
}

// Close is a no-op; the caller manages the *sql.DB lifecycle.
func (s *Storage) Close(context.Context) error {
	return nil
}

// where renders the filter of q as predicates with ? placeholders. Time bounds
// compare whole milliseconds, the precision events are ordered by.
func (s *Storage) where(q *audit.Query) ([]string, []any) {
	var (
		preds []string
		args  []any
	)
	if q.StartTime != nil {
		preds, args = append(preds, "ts_ms >= ?"), append(args, q.StartTime.UnixMilli())
	}
	if q.EndTime != nil {
		preds, args = append(preds, "ts_ms <= ?"), append(args, q.EndTime.UnixMilli())
	}
	for _, f := range []struct{ col, value string }{
		{"actor_id", q.ActorID}, {"actor_type", q.ActorType}, {"resource_type", q.ResourceType}, {"resource_id", q.ResourceID},
		{"event_type", string(q.EventType)}, {"action", string(q.Action)}, {"status", string(q.Status)},
		{"request_id", q.RequestID}, {"trace_id", q.TraceID},
	} {
		if f.value != "" {
			preds, args = append(preds, f.col+" = ?"), append(args, s.dialect.textArg(f.value))
		}
	}
	return preds, args
}

// whereClause joins predicates into a WHERE clause, or "" when there are none.
func whereClause(preds []string) string {
	if len(preds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(preds, " AND ")
}
