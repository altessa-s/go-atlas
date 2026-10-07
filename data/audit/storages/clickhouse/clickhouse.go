// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"context"
	"errors"
	"iter"
	"slices"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/altessa-s/go-atlas/data/audit"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrNoConn is returned by [New] when no ClickHouse connection is supplied.
var ErrNoConn = errors.New("audit/clickhouse: connection is required")

// ErrTimeRangeRequired is returned by [Storage.Query] and [Storage.Count]
// under [TimeRangeModeEnforce] when the query bounds no time range.
var ErrTimeRangeRequired = errors.New("audit/clickhouse: query requires a time range")

// Conn is the subset of driver.Conn the storage needs. Declaring it here
// keeps the connection injectable — the caller owns its lifecycle — and lets
// tests substitute a double without a live server.
type Conn interface {
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) driver.Row
	Exec(ctx context.Context, query string, args ...any) error
}

var (
	_ audit.Storage = (*Storage)(nil)
	_ Conn          = (driver.Conn)(nil)
)

// Storage implements [audit.Storage] on top of ClickHouse.
type Storage struct {
	conn       Conn
	opts       *options
	insertStmt string
}

// New creates a ClickHouse audit storage over an existing connection.
//
// The table, cluster and engine names are validated as [SchemaDDL]
// validates them, failing with [ErrInvalidIdentifier] or [ErrInvalidEngine].
// With [WithAutoCreateTable] the table is created on the spot from
// [SchemaDDL], bounded by [WithDDLTimeout]; otherwise the table is expected
// to exist already, which is where a migration-managed deployment wants it.
func New(conn Conn, opts ...Option) (*Storage, error) {
	if conn == nil {
		return nil, ErrNoConn
	}

	o := newOptions(opts...)
	if err := validateSchemaNames(o.tableName, o.engine, o.cluster); err != nil {
		return nil, err
	}

	s := &Storage{
		conn:       conn,
		opts:       o,
		insertStmt: insertStatement(o.tableName),
	}

	if o.autoCreateTable {
		ddl, err := SchemaDDL(o.tableName, o.engine, o.cluster, o.ttl)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), o.ddlTimeout)
		defer cancel()
		if err := conn.Exec(ctx, ddl); err != nil {
			return nil, coreerrs.WrapOperation(err, "create audit table")
		}
	}

	return s, nil
}

// Store inserts a single audit event.
//
// Single-row INSERTs are the one thing ClickHouse handles badly — each
// creates a part of its own — so the write goes through an asynchronous
// insert, which buffers it server-side into a shared block.
// [Storage.StoreBatch] remains the hot path.
func (s *Storage) Store(ctx context.Context, event *audit.Event) error {
	asyncCtx := chgo.Context(ctx, chgo.WithSettings(chgo.Settings{
		"async_insert":          1,
		"wait_for_async_insert": 1,
	}))

	return s.send(asyncCtx, []*audit.Event{event})
}

// StoreBatch inserts multiple audit events, splitting the input into batches
// of at most [DefaultMaxBatchSize] rows. An empty slice is a no-op.
//
// The batches are not atomic with respect to each other: a failure partway
// through leaves the earlier batches inserted. Callers get at-least-once
// semantics from the dispatcher and dedup from the table engine.
func (s *Storage) StoreBatch(ctx context.Context, events []*audit.Event) error {
	for chunk := range slices.Chunk(events, s.opts.maxBatchSize) {
		if err := s.send(ctx, chunk); err != nil {
			return err
		}
	}

	return nil
}

// send writes one batch of events.
func (s *Storage) send(ctx context.Context, events []*audit.Event) error {
	batch, err := s.conn.PrepareBatch(ctx, s.insertStmt)
	if err != nil {
		return coreerrs.WrapOperation(err, "prepare audit batch")
	}
	defer func() { _ = batch.Close() }()

	// One row and one column buffer for the whole batch: the driver is fed
	// column-wise, which keeps allocations proportional to the number of
	// columns rather than to the number of rows.
	var row eventRow

	buf := newColumnBuffer(len(events))
	for _, event := range events {
		if err := toRowInto(&row, event); err != nil {
			return coreerrs.WrapOperation(err, "convert audit event")
		}

		buf.appendRow(&row)
	}

	if err := buf.flush(batch); err != nil {
		return coreerrs.WrapOperation(err, "append audit events")
	}

	if err := batch.Send(); err != nil {
		return coreerrs.WrapOperation(err, "send audit batch")
	}

	return nil
}

// Query returns an iterator over the events matching query, sorted by event
// time, newest first unless the query asks for ascending order, starting
// after query.Cursor. Event ID breaks ties, so keyset paging stays stable
// even when a batch of events shares a timestamp.
func (s *Storage) Query(ctx context.Context, query *audit.Query) iter.Seq2[*audit.Event, error] {
	if query == nil {
		query = &audit.Query{} // a nil query is a bare listing
	}

	return func(yield func(*audit.Event, error) bool) {
		if err := s.checkTimeRange(ctx, query); err != nil {
			yield(nil, err)
			return
		}

		stmt, args := s.selectStatement(query, query.Cursor)

		rows, err := s.conn.Query(ctx, stmt, args...)
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "query audit events"))
			return
		}
		defer func() { _ = rows.Close() }()

		// scanDest points into row, so both are hoisted out of the loop: one
		// destination slice per query instead of one per row. Zeroing row
		// each iteration keeps the driver from reusing the previous row's
		// slices, so yielded events never alias one another.
		var row eventRow
		dest := row.scanDest()

		for rows.Next() {
			row = eventRow{}

			if err := rows.Scan(dest...); err != nil {
				yield(nil, coreerrs.WrapOperation(err, "scan audit event"))
				return
			}

			event, err := fromRow(&row)
			if err != nil {
				yield(nil, coreerrs.WrapOperation(err, "convert audit event"))
				return
			}
			if !yield(event, nil) {
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
	if err := s.checkTimeRange(ctx, query); err != nil {
		return 0, err
	}

	stmt, args := s.countStatement(query, nil)

	var count int64
	if err := s.conn.QueryRow(ctx, stmt, args...).Scan(&count); err != nil {
		return 0, coreerrs.WrapOperation(err, "count audit events")
	}

	return count, nil
}

// Close is a no-op; the caller owns the underlying connection.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// checkTimeRange applies [TimeRangeMode] to a query that bounds no time
// range and would therefore read every part of the table.
func (s *Storage) checkTimeRange(ctx context.Context, q *audit.Query) error {
	if q != nil && (q.StartTime != nil || q.EndTime != nil) {
		return nil
	}

	if s.opts.timeRangeMode == TimeRangeModeEnforce {
		return ErrTimeRangeRequired
	}

	if s.opts.timeRangeMode == TimeRangeModeWarn {
		s.opts.logger.WarnContext(ctx, "audit query without a time range scans the whole table",
			"table", s.opts.tableName)
	}

	return nil
}
