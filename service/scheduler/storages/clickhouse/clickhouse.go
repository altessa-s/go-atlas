// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"context"
	"errors"
	"iter"
	"maps"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	chfilter "github.com/altessa-s/go-atlas/data/filter/translators/clickhouse"
)

// deleteSyncAllReplicas is the lightweight_deletes_sync value that waits for
// the delete on every replica.
const deleteSyncAllReplicas = 2

// ErrNoConn is returned by [New] when no ClickHouse connection is supplied.
var ErrNoConn = errors.New("scheduler/clickhouse: connection is required")

// Conn is the subset of driver.Conn the storage needs. The caller owns the
// connection and its lifecycle.
type Conn interface {
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
	Exec(ctx context.Context, query string, args ...any) error
}

var (
	_ scheduler.HistoryStorage = (*Storage)(nil)
	_ scheduler.HistoryDeleter = (*Storage)(nil)
	_ Conn                     = (driver.Conn)(nil)
)

// historyFieldMapping maps the CEL field names of [scheduler.HistoryFilterFields]
// to the history columns.
var historyFieldMapping = coremaps.NewImmutableMap(map[string]string{
	"id": "id", "taskId": "task_id", "runId": "run_id", "startedAt": "started_at",
	"endedAt": "ended_at", "durationMs": "duration_ms", "success": "success", "error": "error",
})

// Storage implements [scheduler.HistoryStorage] on a ClickHouse table, for use
// with [scheduler.WithHistoryStorage]. It is safe for concurrent use.
type Storage struct {
	conn  Conn
	opts  *options
	table string // backtick-quoted table name

	insertStmt, historyStmt, deleteStmt string
}

// New creates a ClickHouse history storage over an existing connection. It
// performs no I/O: the table is expected to exist, created by
// [Storage.EnsureSchema] or applied from [SchemaDDL] through a migration. The
// table, cluster and engine names are validated as [SchemaDDL] validates them.
func New(conn Conn, opts ...Option) (*Storage, error) {
	if conn == nil {
		return nil, ErrNoConn
	}
	o := newOptions(opts...)
	if err := validateSchemaNames(o.tableName, o.engine, o.cluster); err != nil {
		return nil, err
	}
	table := "`" + o.tableName + "`"
	return &Storage{
		conn:        conn,
		opts:        o,
		table:       table,
		insertStmt:  "INSERT INTO " + table + " (" + historyColumns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		historyStmt: "SELECT " + historyColumns + " FROM " + table + " WHERE `task_id` = ? ORDER BY `started_at` DESC, `id` DESC",
		deleteStmt:  "DELETE FROM " + table + onCluster(o.cluster) + " WHERE `task_id` = ?",
	}, nil
}

// EnsureSchema creates the history table from [SchemaDDL] unless it exists. A
// table that exists is left as it is, whatever its definition.
func (s *Storage) EnsureSchema(ctx context.Context) error {
	ddl, err := SchemaDDL(s.opts.tableName, s.opts.engine, s.opts.cluster, s.opts.ttl)
	if err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, ddl); err != nil {
		return coreerrs.WrapOperation(err, "create scheduler history table")
	}
	return nil
}

// AddHistory inserts one history entry. The scheduler writes one row per run,
// which ClickHouse handles badly as a part of its own, so the row goes through
// an asynchronous insert that the server buffers into a shared block. The call
// waits for the flush, so the entry is visible once it returns.
func (s *Storage) AddHistory(ctx context.Context, h *scheduler.TaskHistory) error {
	asyncCtx := chgo.Context(ctx, chgo.WithSettings(chgo.Settings{
		"async_insert":          1,
		"wait_for_async_insert": 1,
	}))
	if err := s.conn.Exec(asyncCtx, s.insertStmt,
		h.ID, h.TaskID, h.RunID, h.Error, h.StartedAt, h.EndedAt, h.DurationMs, h.Success); err != nil {
		return coreerrs.WrapOperation(err, "add scheduler history")
	}
	return nil
}

// History yields the task's history ordered by StartedAt descending, ID
// descending as tie-breaker — the order [Storage.HistoryPaginated] uses.
func (s *Storage) History(ctx context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return s.query(ctx, s.historyStmt, id)
}

// CleanupHistory is a no-op: the table TTL rendered by [SchemaDDL] deletes the
// entries past their retention as ClickHouse merges parts. The TTL is fixed
// when the table is created, so retention is the [WithTTL] of that moment, not
// the retention passed here; change it with ALTER TABLE … MODIFY TTL. An
// expired entry stays visible until the merge that drops it.
func (s *Storage) CleanupHistory(context.Context, time.Duration) error {
	return nil
}

// DeleteHistory removes the history of task id with a lightweight DELETE,
// which marks the rows deleted and purges them as parts merge. The statement
// sets lightweight_deletes_sync = 2, so it returns only once every replica
// hides the rows, whatever the session or profile default; the setting
// requires ClickHouse 24.x.
func (s *Storage) DeleteHistory(ctx context.Context, id string) error {
	syncCtx := chgo.Context(ctx, chgo.WithSettings(chgo.Settings{
		"lightweight_deletes_sync": deleteSyncAllReplicas,
	}))
	if err := s.conn.Exec(syncCtx, s.deleteStmt, id); err != nil {
		return coreerrs.WrapOperation(err, "delete scheduler history")
	}
	return nil
}

// HistoryPaginated returns up to pg.Limit+1 history entries of taskID after the
// cursor, ordered by StartedAt descending and ID descending. A non-nil f is
// evaluated by ClickHouse.
func (s *Storage) HistoryPaginated(ctx context.Context, taskID string, pg scheduler.HistoryPagination, f filter.Node) ([]*scheduler.TaskHistory, error) {
	stmt, args, err := s.pageStatement(taskID, pg, f)
	if err != nil {
		return nil, err
	}
	var page []*scheduler.TaskHistory
	for h, err := range s.query(ctx, stmt, args...) {
		if err != nil {
			return nil, err
		}
		page = append(page, h)
	}
	return page, nil
}

// pageStatement renders the HistoryPaginated query and its arguments, in the
// order of their placeholders. Translators are not safe for concurrent use, so
// one is built per call.
func (s *Storage) pageStatement(taskID string, pg scheduler.HistoryPagination, f filter.Node) (string, []any, error) {
	var b strings.Builder
	b.WriteString("SELECT " + historyColumns + " FROM " + s.table + " WHERE `task_id` = ?")
	args := []any{taskID}
	if pg.AfterID != "" {
		b.WriteString(" AND (`started_at` < ? OR (`started_at` = ? AND `id` < ?))")
		args = append(args, pg.AfterStartedAt, pg.AfterStartedAt, pg.AfterID)
	}
	if f != nil {
		trans, err := chfilter.NewTranslator(
			filter.WithAllowedFields(scheduler.HistoryFilterFields...),
			filter.WithFieldMapping(maps.Collect(historyFieldMapping.All())),
		)
		if err != nil {
			return "", nil, err
		}
		where, filterArgs, err := trans.Translate(f)
		if err != nil {
			return "", nil, err
		}
		b.WriteString(" AND (" + where + ")")
		args = append(args, filterArgs...)
	}
	b.WriteString(" ORDER BY `started_at` DESC, `id` DESC LIMIT ?")
	return b.String(), append(args, pg.Limit+1), nil
}

// query yields the history rows stmt selects.
func (s *Storage) query(ctx context.Context, stmt string, args ...any) iter.Seq2[*scheduler.TaskHistory, error] {
	return func(yield func(*scheduler.TaskHistory, error) bool) {
		rows, err := s.conn.Query(ctx, stmt, args...)
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "query scheduler history"))
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			h := new(scheduler.TaskHistory)
			if err := rows.Scan(&h.ID, &h.TaskID, &h.RunID, &h.Error, &h.StartedAt, &h.EndedAt, &h.DurationMs, &h.Success); err != nil {
				yield(nil, coreerrs.WrapOperation(err, "scan scheduler history"))
				return
			}
			if !yield(h, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, coreerrs.WrapOperation(err, "iterate scheduler history"))
		}
	}
}
