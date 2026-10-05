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
	"maps"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/service/scheduler"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// taskColumns is the column list of the tasks table in scan order.
const taskColumns = "id, description, status, priority, schedule, last_run_at, next_run_at, last_run_id, run_started_at, " +
	"run_lease_until, run_lease_id, run_at, failures, skip_next_run, disable_history, unmanaged, one_shot, meta, created_at, updated_at, revision"

// historyColumns is the column list of the history table in scan order.
const historyColumns = "id, task_id, run_id, error, started_at, ended_at, duration_ms, success"

// taskFieldMapping maps CEL field names (camelCase) to task columns.
var taskFieldMapping = coremaps.NewImmutableMap(map[string]string{
	"lastRunAt": "last_run_at", "nextRunAt": "next_run_at", "skipNextRun": "skip_next_run",
	"disableHistory": "disable_history", "oneShot": "one_shot",
})

// historyFieldMapping maps CEL field names (camelCase) to history columns.
var historyFieldMapping = coremaps.NewImmutableMap(map[string]string{
	"taskId": "task_id", "runId": "run_id", "startedAt": "started_at",
	"endedAt": "ended_at", "durationMs": "duration_ms",
})

// Storage implements [scheduler.Storage] on a SQL database through the
// standard database/sql package. The caller owns the *sql.DB and chooses the
// driver; the storage only needs the matching [Dialect].
//
// Every write that the contract requires to be atomic — revision increments,
// CreateTask, ClaimRun, RenewRun, FinishRun, ReplaceTaskIf — is a single
// statement, conditional where the contract fences it, so the database row lock
// serializes concurrent writers. Methods are safe for concurrent use.
type Storage struct {
	db      *sql.DB
	dialect dialect

	tasksTable, historyTable string // quoted
	tasksName, historyName   string // unqualified, for index names
	tasksSchema              string // schema qualifier of the tasks table, if any

	stmts statements

	taskFilter, historyFilter filterSource
}

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	getTask, upsertTask, createTask, replaceTask string
	claim, claimAny, renew, finish               string
	deleteHistory, deleteTask, tasks, dueTasks   string
	addHistory, history, cleanupHistory          string
}

var _ scheduler.Storage = (*Storage)(nil)

// New creates a [Storage] over db for the given dialect. It performs no I/O;
// call [Storage.EnsureSchema] once at startup to create the tables, or apply
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
//	s := scheduler.New(storage)
func New(db *sql.DB, d Dialect, opts ...Option) (*Storage, error) {
	if db == nil {
		return nil, errors.New("sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)

	s := &Storage{db: db, dialect: dl}
	if s.tasksTable, err = dl.Table(o.tasksTable); err != nil {
		return nil, err
	}
	if s.historyTable, err = dl.Table(o.historyTable); err != nil {
		return nil, err
	}
	s.tasksName = sqldialect.Unqualified(o.tasksTable)
	s.tasksSchema = sqldialect.Qualifier(o.tasksTable)
	s.historyName = sqldialect.Unqualified(o.historyTable)
	s.stmts = s.buildStatements()
	s.taskFilter, s.historyFilter = s.filterSources()
	return s, nil
}

func (s *Storage) buildStatements() statements {
	t, h, b := s.tasksTable, s.historyTable, s.dialect.Bind
	values := "?" + strings.Repeat(", ?", strings.Count(taskColumns, ",")-1) + ", 1"

	// assign renders "c = <value of c>" for every mutable column, joined.
	mutable := mutableTaskColumns()
	assign := func(value func(col string) string) string {
		set := make([]string, len(mutable))
		for i, c := range mutable {
			set[i] = c + " = " + value(c)
		}
		return strings.Join(set, ", ")
	}

	insert := "INSERT INTO " + t + " (" + taskColumns + ") VALUES (" + values + ")"
	var upsert, create string
	switch s.dialect.name {
	case DialectPostgres:
		upsert = "INSERT INTO " + t + " AS cur (" + taskColumns + ") VALUES (" + values + ") ON CONFLICT (id) DO UPDATE SET " +
			assign(func(c string) string { return "EXCLUDED." + c }) + ", revision = cur.revision + 1"
		create = insert + " ON CONFLICT (id) DO NOTHING"
	case DialectMySQL:
		upsert = insert + " ON DUPLICATE KEY UPDATE " +
			assign(func(c string) string { return "VALUES(" + c + ")" }) + ", revision = revision + 1"
		// MySQL has no portable insert-if-absent whose affected-rows count is
		// unambiguous under every client flag (INSERT IGNORE also downgrades
		// other errors); CreateTask runs a plain INSERT and resolves a failure by
		// checking whether the row exists.
		create = insert
	}
	replaceSet := assign(func(string) string { return "?" })

	// Status constants are inlined in FinishRun: a bare parameter as a CASE
	// result has no type PostgreSQL can infer.
	running := strconv.Itoa(int(scheduler.TaskStatusRunning))
	completed := strconv.Itoa(int(scheduler.TaskStatusCompleted))
	active := strconv.Itoa(int(scheduler.TaskStatusActive))

	// claimSet stamps the run and its first lease. The claimable predicate:
	// active and no unfinished run (run_started_at = 0), optionally fenced on
	// the occurrence (next_run_at and run_at).
	const claimSet = " SET status = ?, run_started_at = ?, last_run_id = ?, run_lease_until = ?, run_lease_id = ?, updated_at = ?," +
		" revision = revision + 1 WHERE id = ? AND status = ? AND run_started_at = 0"
	// owned is the ownership predicate shared by RenewRun and FinishRun.
	const owned = " WHERE id = ? AND last_run_id = ? AND run_started_at <> 0"
	// executed: a one-shot task still registered for the occurrence the run
	// executed (re-registration may have moved it to another run_at meanwhile).
	const executed = "(one_shot AND run_at = ?)"
	return statements{
		getTask:    b("SELECT "+taskColumns+" FROM "+t+" WHERE id = ?", 1),
		upsertTask: b(upsert, 1),
		createTask: b(create, 1),
		replaceTask: b("UPDATE "+t+" SET "+replaceSet+", revision = ?"+
			" WHERE id = ? AND status = ? AND next_run_at = ? AND last_run_id = ? AND run_started_at = ? AND revision = ?", 1),
		claim:    b("UPDATE "+t+claimSet+" AND next_run_at = ? AND run_at = ?", 1),
		claimAny: b("UPDATE "+t+claimSet, 1),
		renew:    b("UPDATE "+t+" SET run_lease_until = ?, run_lease_id = ?, revision = revision + 1"+owned, 1),
		// MySQL applies SET assignments left to right and later ones see the
		// new values. No expression here reads a column assigned before it, so
		// both dialects evaluate every expression against the old row.
		finish: b("UPDATE "+t+" SET"+
			" next_run_at = CASE WHEN "+executed+" THEN 0 WHEN one_shot THEN next_run_at WHEN schedule = ? THEN ? ELSE next_run_at END,"+
			" status = CASE WHEN "+executed+" THEN "+completed+" WHEN status = "+running+" THEN "+active+" ELSE status END,"+
			" last_run_at = ?, updated_at = ?, run_started_at = 0, run_lease_until = 0, run_lease_id = '',"+
			" failures = CASE WHEN ? THEN 0 ELSE failures + 1 END,"+
			" revision = revision + 1"+owned, 1),
		deleteHistory:  b("DELETE FROM "+h+" WHERE task_id = ?", 1),
		deleteTask:     b("DELETE FROM "+t+" WHERE id = ?", 1),
		tasks:          "SELECT " + taskColumns + " FROM " + t + " ORDER BY id",
		dueTasks:       b("SELECT "+taskColumns+" FROM "+t+" WHERE status = ? AND next_run_at <= ? ORDER BY id", 1),
		addHistory:     b("INSERT INTO "+h+" ("+historyColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)", 1),
		history:        b("SELECT "+historyColumns+" FROM "+h+" WHERE task_id = ? ORDER BY started_at DESC, id DESC", 1),
		cleanupHistory: b("DELETE FROM "+h+" WHERE ended_at < ?", 1),
	}
}

// mutableTaskColumns lists every task column except the key and the revision.
func mutableTaskColumns() []string {
	cols := strings.Split(taskColumns, ", ")
	return cols[1 : len(cols)-1]
}

// GetTask returns the task with the given id, or (nil, nil) when it does not
// exist.
func (s *Storage) GetTask(ctx context.Context, id string) (*scheduler.TaskState, error) {
	state, err := scanTask(s.db.QueryRowContext(ctx, s.stmts.getTask, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil state with nil error indicates "not found"
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get scheduler task")
	}
	return state, nil
}

// UpsertTask inserts or fully replaces the task identified by state.ID in a
// single statement that also sets the revision to the stored value plus one
// (one for a new task); the caller's Revision is ignored.
func (s *Storage) UpsertTask(ctx context.Context, state *scheduler.TaskState) error {
	if err := checkTask(state); err != nil {
		return err
	}
	args, err := taskArgs(state)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.stmts.upsertTask, append([]any{state.ID}, args...)...); err != nil {
		return coreerrs.WrapOperation(err, "upsert scheduler task")
	}
	return nil
}

// ReplaceTaskIf replaces the task only while every fenced column still equals
// expect, in one conditional UPDATE that stores expect.Revision+1.
func (s *Storage) ReplaceTaskIf(ctx context.Context, state *scheduler.TaskState, expect scheduler.TaskFence) (bool, error) {
	if err := checkTask(state); err != nil {
		return false, err
	}
	args, err := taskArgs(state)
	if err != nil {
		return false, err
	}
	args = append(args, expect.Revision+1, state.ID,
		int32(expect.Status), expect.NextRunAt, expect.LastRunID, expect.RunStartedAt, expect.Revision)
	return s.affected(ctx, "replace scheduler task", s.stmts.replaceTask, args...)
}

// CreateTask inserts the task with revision one only when no task with its ID
// exists. On PostgreSQL the insert is ON CONFLICT DO NOTHING; on MySQL a
// failed INSERT is reported as (false, nil) when the task exists — a duplicate
// key — and as the error otherwise.
func (s *Storage) CreateTask(ctx context.Context, state *scheduler.TaskState) (bool, error) {
	if err := checkTask(state); err != nil {
		return false, err
	}
	args, err := taskArgs(state)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, s.stmts.createTask, append([]any{state.ID}, args...)...)
	if err != nil {
		if s.dialect.name == DialectMySQL {
			if existing, getErr := s.GetTask(ctx, state.ID); getErr == nil && existing != nil {
				return false, nil
			}
		}
		return false, coreerrs.WrapOperation(err, "create scheduler task")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "create scheduler task")
	}
	return n == 1, nil
}

// ClaimRun transitions the task from active to running while it is claimable —
// no unfinished run and, when claim.NextRunAt is non-zero, the claimed
// occurrence (next_run_at and run_at) still stored — in one conditional
// UPDATE that also stores the run's first lease; the caller won iff a row was
// updated.
func (s *Storage) ClaimRun(ctx context.Context, id string, claim scheduler.RunClaim) (bool, error) {
	if err := claim.Validate(); err != nil {
		return false, err
	}
	if err := checkLength(MaxIDLength, claim.RunID); err != nil {
		return false, err
	}
	args := []any{int32(scheduler.TaskStatusRunning), claim.StartedAt, claim.RunID, claim.LeaseUntil, claim.RunID, claim.StartedAt,
		id, int32(scheduler.TaskStatusActive)}
	if claim.NextRunAt == 0 {
		return s.affected(ctx, "claim scheduler task", s.stmts.claimAny, args...)
	}
	return s.affected(ctx, "claim scheduler task", s.stmts.claim, append(args, claim.NextRunAt, claim.RunAt)...)
}

// RenewRun extends the lease of the unfinished run runID and binds it to the
// run, in one conditional UPDATE.
func (s *Storage) RenewRun(ctx context.Context, id, runID string, leaseUntil int64) (bool, error) {
	if runID == "" {
		return false, nil
	}
	return s.affected(ctx, "renew scheduler run", s.stmts.renew, leaseUntil, runID, id, runID)
}

// FinishRun applies the finish transition only while runID still owns an
// unfinished run, preserving concurrent pause/disable, schedule and metadata
// changes and clearing the run's lease.
func (s *Storage) FinishRun(ctx context.Context, id, runID string, result scheduler.RunResult) (bool, error) {
	if runID == "" {
		return false, nil
	}
	return s.affected(ctx, "finish scheduler run", s.stmts.finish,
		result.RunAt, result.Schedule, result.NextRunAt,
		result.RunAt,
		result.StartedAt, result.EndedAt,
		result.Success,
		id, runID,
	)
}

// affected runs a conditional write and reports whether it updated one row.
// Every conditional write here changes the revision column, so a matched row
// is always a changed row — also under MySQL's changed-rows semantics.
func (s *Storage) affected(ctx context.Context, op, query string, args ...any) (bool, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, coreerrs.WrapOperation(err, op)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, coreerrs.WrapOperation(err, op)
	}
	return n == 1, nil
}

// DeleteTask removes the task and its history in one transaction. Deleting a
// missing task is not an error.
func (s *Storage) DeleteTask(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreerrs.WrapOperation(err, "delete scheduler task")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.stmts.deleteHistory, id); err != nil {
		return coreerrs.WrapOperation(err, "delete scheduler task history")
	}
	if _, err := tx.ExecContext(ctx, s.stmts.deleteTask, id); err != nil {
		return coreerrs.WrapOperation(err, "delete scheduler task")
	}
	return coreerrs.WrapOperation(tx.Commit(), "delete scheduler task")
}

// Tasks yields every task ordered by ID. The query runs on first iteration and
// its rows are closed on exhaustion or early break.
func (s *Storage) Tasks(ctx context.Context) iter.Seq2[*scheduler.TaskState, error] {
	return queryIter(ctx, s.db, s.stmts.tasks, nil, scanTask)
}

// DueTasks yields the active tasks with next_run_at <= now ordered by ID; the
// predicate is served by the (status, next_run_at) index.
func (s *Storage) DueTasks(ctx context.Context, now int64) iter.Seq2[*scheduler.TaskState, error] {
	return queryIter(ctx, s.db, s.stmts.dueTasks, []any{int32(scheduler.TaskStatusActive), now}, scanTask)
}

// AddHistory inserts one history entry.
func (s *Storage) AddHistory(ctx context.Context, h *scheduler.TaskHistory) error {
	if err := checkLength(MaxIDLength, h.ID, h.TaskID, h.RunID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.stmts.addHistory,
		h.ID, h.TaskID, h.RunID, h.Error, h.StartedAt, h.EndedAt, h.DurationMs, h.Success); err != nil {
		return coreerrs.WrapOperation(err, "add scheduler history")
	}
	return nil
}

// History yields the task's history ordered by StartedAt descending, ID
// descending as tie-breaker — the order [Storage.HistoryPaginated] uses.
func (s *Storage) History(ctx context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return queryIter(ctx, s.db, s.stmts.history, []any{id}, scanHistory)
}

// CleanupHistory deletes history entries that ended before now minus
// retention (Unix seconds, like every scheduler timestamp).
func (s *Storage) CleanupHistory(ctx context.Context, retention time.Duration) error {
	if _, err := s.db.ExecContext(ctx, s.stmts.cleanupHistory, time.Now().Add(-retention).Unix()); err != nil {
		return coreerrs.WrapOperation(err, "clean up scheduler history")
	}
	return nil
}

// TasksPaginated returns up to pg.Limit+1 tasks with ID greater than
// pg.AfterID, ordered by ID. A non-nil f is translated to a parameterized SQL
// predicate and evaluated by the database.
func (s *Storage) TasksPaginated(ctx context.Context, pg scheduler.Pagination, f filter.Node) ([]*scheduler.TaskState, error) {
	from, where, args, err := s.filterClause(f, scheduler.TaskFilterFields, s.tasksTable, s.taskFilter)
	if err != nil {
		return nil, err
	}
	next := len(args) + 1 // the filter owns placeholders 1..len(args)
	own := ""
	if pg.AfterID != "" {
		own = " AND id > ?"
		args = append(args, pg.AfterID)
	}
	query := "SELECT " + taskColumns + " FROM " + from + " WHERE " + where +
		s.dialect.Bind(own+" ORDER BY id LIMIT ?", next)
	return collect(queryIter(ctx, s.db, query, append(args, pg.Limit+1), scanTask))
}

// HistoryPaginated returns up to pg.Limit+1 history entries of taskID after the
// cursor, ordered by StartedAt descending and ID descending. A non-nil f is
// evaluated by the database.
func (s *Storage) HistoryPaginated(ctx context.Context, taskID string, pg scheduler.HistoryPagination, f filter.Node) ([]*scheduler.TaskHistory, error) {
	from, where, args, err := s.filterClause(f, scheduler.HistoryFilterFields, s.historyTable, s.historyFilter)
	if err != nil {
		return nil, err
	}
	next := len(args) + 1 // the filter owns placeholders 1..len(args)
	own := " AND task_id = ?"
	args = append(args, taskID)
	if pg.AfterID != "" {
		own += " AND (started_at < ? OR (started_at = ? AND id < ?))"
		args = append(args, pg.AfterStartedAt, pg.AfterStartedAt, pg.AfterID)
	}
	query := "SELECT " + historyColumns + " FROM " + from + " WHERE " + where +
		s.dialect.Bind(own+" ORDER BY started_at DESC, id DESC LIMIT ?", next)
	return collect(queryIter(ctx, s.db, query, append(args, pg.Limit+1), scanHistory))
}

// filterClause renders f as a parenthesized predicate whose placeholders start
// at 1, so the caller's own predicates follow it textually and numerically,
// and returns the relation to read from: table when f is nil (the predicate is
// then always true), src.from otherwise. Translators are not safe for
// concurrent use, so one is built per call.
func (s *Storage) filterClause(f filter.Node, fields []string, table string, src filterSource) (string, string, []any, error) {
	if f == nil {
		return table, "1 = 1", nil, nil
	}
	trans, err := s.dialect.newTranslator(
		filter.WithAllowedFields(fields...),
		filter.WithFieldMapping(maps.Collect(src.mapping.All())),
	)
	if err != nil {
		return "", "", nil, err
	}
	where, args, err := trans.Translate(f)
	if err != nil {
		return "", "", nil, err
	}
	return src.from, "(" + where + ")", args, nil
}

// filterSource is the relation a filtered page query reads from and the
// mapping of CEL field names onto its columns.
type filterSource struct {
	from    string
	mapping *coremaps.ImmutableMap[string, string]
}

// filterSources returns the filter relations of the tasks and history tables.
//
// On PostgreSQL they are the tables themselves. On MySQL/MariaDB the string
// columns are binary types, on which the MariaDB translator's CHAR_LENGTH and
// RIGHT count bytes rather than characters, and MySQL rejects REGEXP outright.
// A filtered query therefore reads from a derived table that adds a utf8mb4
// text view of every filterable string column (text_<column>, case-sensitive
// utf8mb4_bin), and the CEL string fields map onto those views. Pagination,
// ordering and the task_id predicate stay on the raw columns, so the engines
// merge the derived table into the outer query and keep using the indexes.
// utf8mb4_bin is PAD SPACE — the only binary utf8mb4 collation common to every
// supported server — so filter comparisons on these fields ignore trailing
// spaces; the storage's own lookups and fences compare the raw columns exactly.
func (s *Storage) filterSources() (tasks, history filterSource) {
	if s.dialect.name != DialectMySQL {
		return filterSource{from: s.tasksTable, mapping: taskFieldMapping},
			filterSource{from: s.historyTable, mapping: historyFieldMapping}
	}
	return textView(s.tasksTable, taskColumns, taskFieldMapping, [][2]string{{"id", "id"}, {"description", "description"}, {"schedule", "schedule"}}),
		textView(s.historyTable, historyColumns, historyFieldMapping, [][2]string{{"id", "id"}, {"taskId", "task_id"}, {"runId", "run_id"}, {"error", "error"}})
}

// textView builds a MySQL derived table over table that selects columns plus a
// text view of each string column in text, given as (CEL field, column) pairs,
// and maps those fields onto the views.
func textView(table, columns string, base *coremaps.ImmutableMap[string, string], text [][2]string) filterSource {
	mapping := maps.Collect(base.All())
	var b strings.Builder
	b.WriteString("(SELECT " + columns)
	for _, fc := range text {
		alias := "text_" + fc[1]
		b.WriteString(", CONVERT(" + fc[1] + " USING utf8mb4) COLLATE utf8mb4_bin AS " + alias)
		mapping[fc[0]] = alias
	}
	b.WriteString(" FROM " + table + ") AS f")
	return filterSource{from: b.String(), mapping: coremaps.NewImmutableMap(mapping)}
}

// checkTask rejects a task whose ID-like fields exceed their columns.
func checkTask(state *scheduler.TaskState) error {
	if err := checkLength(MaxIDLength, state.ID, state.LastRunID, state.RunLeaseID); err != nil {
		return err
	}
	return checkLength(MaxScheduleLength, state.Schedule)
}

// checkLength reports [ErrValueTooLong] when any value has more than limit
// characters.
func checkLength(limit int, values ...string) error {
	for _, v := range values {
		if n := utf8.RuneCountInString(v); n > limit {
			return fmt.Errorf("%w: %d characters, limit %d", ErrValueTooLong, n, limit)
		}
	}
	return nil
}

// taskArgs returns the mutable column values of state in taskColumns order.
func taskArgs(state *scheduler.TaskState) ([]any, error) {
	meta := []byte("{}")
	if len(state.Meta) > 0 {
		var err error
		if meta, err = json.Marshal(state.Meta); err != nil {
			return nil, coreerrs.WrapOperation(err, "encode scheduler task meta")
		}
	}
	return []any{
		state.Description, int32(state.Status), int32(state.Priority), state.Schedule,
		state.LastRunAt, state.NextRunAt, state.LastRunID, state.RunStartedAt,
		state.RunLeaseUntil, state.RunLeaseID, state.RunAt, state.Failures,
		state.SkipNextRun, state.DisableHistory, state.Unmanaged, state.OneShot,
		string(meta), state.CreatedAt, state.UpdatedAt,
	}, nil
}

// rowScanner is the common part of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(r rowScanner) (*scheduler.TaskState, error) {
	var (
		t                scheduler.TaskState
		status, priority int32
		meta             string
	)
	if err := r.Scan(&t.ID, &t.Description, &status, &priority, &t.Schedule, &t.LastRunAt, &t.NextRunAt, &t.LastRunID,
		&t.RunStartedAt, &t.RunLeaseUntil, &t.RunLeaseID, &t.RunAt, &t.Failures, &t.SkipNextRun, &t.DisableHistory,
		&t.Unmanaged, &t.OneShot, &meta, &t.CreatedAt, &t.UpdatedAt, &t.Revision); err != nil {
		return nil, err
	}
	t.Status = scheduler.TaskStatus(status)
	t.Priority = scheduler.TaskPriority(priority)
	if meta != "" && meta != "{}" {
		if err := json.Unmarshal([]byte(meta), &t.Meta); err != nil {
			return nil, coreerrs.WrapOperation(err, "decode scheduler task meta")
		}
	}
	return &t, nil
}

func scanHistory(r rowScanner) (*scheduler.TaskHistory, error) {
	var h scheduler.TaskHistory
	if err := r.Scan(&h.ID, &h.TaskID, &h.RunID, &h.Error, &h.StartedAt, &h.EndedAt, &h.DurationMs, &h.Success); err != nil {
		return nil, err
	}
	return &h, nil
}

// queryIter runs query lazily on first iteration and yields scanned rows. Rows
// are closed on exhaustion, error or early break; errors are yielded inline and
// end the iteration.
func queryIter[T any](ctx context.Context, db *sql.DB, query string, args []any, scan func(rowScanner) (*T, error)) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "query scheduler storage"))
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			v, err := scan(rows)
			if err != nil {
				yield(nil, coreerrs.WrapOperation(err, "scan scheduler row"))
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, coreerrs.WrapOperation(err, "read scheduler rows"))
		}
	}
}

func collect[T any](seq iter.Seq2[*T, error]) ([]*T, error) {
	var out []*T
	for v, err := range seq {
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
