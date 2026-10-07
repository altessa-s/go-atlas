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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// columns is the column list of the instances table in scan order.
const columns = "id, definition, status, stage, pending_steps, data, steps, created_at, updated_at, deadline, " +
	"lease_owner, lease_until, version, last_error"

// Store is a durable [saga.Storage] backed by a SQL table. Each saga instance
// is one row keyed by its ID; the row's version column is the
// optimistic-concurrency token ([saga.Instance.Version]), so two coordinators
// cannot advance the same instance — the loser's Update fails with
// [sagaerrs.ErrVersionConflict]. Methods are safe for concurrent use.
type Store struct {
	db        *sql.DB
	dialect   dialect
	table     string // quoted
	tableName string // unqualified, for index names and the DDL lock
	stmts     statements
}

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	get, exists, create, update, remove string
	recoverable, recoverableLimit       string
}

var _ saga.Storage = (*Store)(nil)

// New creates a [Store] over db for the given dialect. It performs no I/O;
// call [Store.EnsureSchema] once at startup to create the table, or apply the
// equivalent DDL through a migration tool.
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	store, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := store.EnsureSchema(ctx); err != nil {
//		return err
//	}
func New(db *sql.DB, d Dialect, opts ...Option) (*Store, error) {
	if db == nil {
		return nil, errors.New("sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)

	s := &Store{db: db, dialect: dl, tableName: sqldialect.Unqualified(o.tableName)}
	if s.table, err = dl.Table(o.tableName); err != nil {
		return nil, err
	}
	s.stmts = s.buildStatements()
	return s, nil
}

func (s *Store) buildStatements() statements {
	t, b := s.table, s.dialect.Bind
	insert := "INSERT INTO " + t + " (" + columns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	create := insert
	if s.dialect.name == DialectPostgres {
		create += " ON CONFLICT (id) DO NOTHING"
	}
	// MySQL has no portable insert-if-absent whose affected-rows count is
	// unambiguous under every client flag (INSERT IGNORE also downgrades other
	// errors); Create runs a plain INSERT and classifies its duplicate-key
	// error.

	// The recovery predicate of saga.Instance.Recoverable: non-terminal, no
	// active lease, and interrupted compensation, an owner whose lease has
	// expired, or a passed deadline. A zero lease_until or deadline is 0.
	recoverable := "SELECT " + columns + " FROM " + t +
		" WHERE status IN (?, ?) AND lease_until <= ?" +
		" AND (status = ? OR lease_owner <> '' OR (deadline <> 0 AND deadline <= ?))"
	return statements{
		get:    b("SELECT "+columns+" FROM "+t+" WHERE id = ?", 1),
		exists: b("SELECT 1 FROM "+t+" WHERE id = ?", 1),
		create: b(create, 1),
		update: b("UPDATE "+t+" SET definition = ?, status = ?, stage = ?, pending_steps = ?, lease_owner = ?, lease_until = ?,"+
			" data = ?, steps = ?, created_at = ?, updated_at = ?, deadline = ?, last_error = ?, version = ?"+
			" WHERE id = ? AND version = ?", 1),
		remove:           b("DELETE FROM "+t+" WHERE id = ?", 1),
		recoverable:      b(recoverable, 1),
		recoverableLimit: b(recoverable+" LIMIT ?", 1),
	}
}

// Create inserts a new instance, returning [sagaerrs.ErrInstanceExists] when a
// row with the same ID already exists. On PostgreSQL the insert is ON CONFLICT
// DO NOTHING; on MySQL a plain INSERT whose error is a duplicate key (error
// 1062) is reported as ErrInstanceExists and every other error as is.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	if n := utf8.RuneCountInString(inst.ID); n > MaxIDLength {
		return fmt.Errorf("%w: %d characters, limit %d", ErrValueTooLong, n, MaxIDLength)
	}
	r, err := s.encode(inst)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, s.stmts.create, s.dialect.textArg(inst.ID), s.dialect.textArg(inst.Definition),
		r.status, int64(inst.Stage), r.pendingSteps, r.data, r.steps, unixNano(inst.CreatedAt), r.updatedAt, r.deadline,
		r.leaseOwner, r.leaseUntil, inst.Version, r.lastError)
	if err != nil {
		if s.dialect.name == DialectMySQL && isDuplicateKey(err) {
			return sagaerrs.ErrInstanceExists
		}
		return coreerrs.WrapOperation(err, "create saga instance in SQL")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return coreerrs.WrapOperation(err, "create saga instance in SQL")
	}
	if n == 0 {
		return sagaerrs.ErrInstanceExists
	}
	return nil
}

// Get loads an instance by ID, returning [sagaerrs.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	inst, err := scanInstance(s.db.QueryRowContext(ctx, s.stmts.get, s.dialect.textArg(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sagaerrs.ErrInstanceNotFound
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get saga instance from SQL")
	}
	return inst, nil
}

// Update overwrites the instance in one UPDATE conditional on the version. A
// stale inst.Version (another coordinator advanced the instance) yields
// [sagaerrs.ErrVersionConflict]; a missing instance yields
// [sagaerrs.ErrInstanceNotFound]. The new version is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	r, err := s.encode(inst)
	if err != nil {
		return err
	}
	newVersion := inst.Version + 1
	res, err := s.db.ExecContext(ctx, s.stmts.update, s.dialect.textArg(inst.Definition), r.status, int64(inst.Stage),
		r.pendingSteps, r.leaseOwner, r.leaseUntil, r.data, r.steps, unixNano(inst.CreatedAt), r.updatedAt, r.deadline, r.lastError,
		newVersion, s.dialect.textArg(inst.ID), inst.Version)
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in SQL")
	}
	// The version always changes, so a matched row is a changed row whatever
	// the MySQL client's found-rows flag.
	n, err := res.RowsAffected()
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in SQL")
	}
	if n == 0 {
		// No row matched {id, version}: either the instance is gone, or its
		// version moved on.
		found, err := s.exists(ctx, inst.ID)
		if err != nil {
			return coreerrs.WrapOperation(err, "disambiguate saga update in SQL")
		}
		if !found {
			return sagaerrs.ErrInstanceNotFound
		}
		return sagaerrs.ErrVersionConflict
	}
	inst.Version = newVersion
	return nil
}

// FetchRecoverable returns up to limit instances that satisfy
// [saga.Instance.Recoverable] at now, compared at full precision. A
// non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*saga.Instance, error) {
	running, compensating := s.dialect.textArg(string(saga.StatusRunning)), s.dialect.textArg(string(saga.StatusCompensating))
	at := unixNano(now)
	query, args := s.stmts.recoverable, []any{running, compensating, at, compensating, at}
	if limit > 0 {
		query, args = s.stmts.recoverableLimit, append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "fetch recoverable saga instances from SQL")
	}
	defer func() { _ = rows.Close() }()

	var out []*saga.Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "decode recoverable saga instance from SQL")
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, coreerrs.WrapOperation(err, "fetch recoverable saga instances from SQL")
	}
	return out, nil
}

// Delete removes an instance. Deleting a missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, s.stmts.remove, s.dialect.textArg(id)); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from SQL")
	}
	return nil
}

// mysqlDuplicateKey is the text MySQL and MariaDB drivers render for error
// 1062 (ER_DUP_ENTRY): go-sql-driver/mysql formats a server error as
// "Error 1062 (23000): Duplicate entry …". database/sql exposes no portable
// error code, so the code is read from the message.
const mysqlDuplicateKey = "Error 1062"

// isDuplicateKey reports whether err is a MySQL duplicate-key error.
func isDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), mysqlDuplicateKey)
}

// exists reports whether a row with the given ID is stored.
func (s *Store) exists(ctx context.Context, id string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, s.stmts.exists, s.dialect.textArg(id)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// row holds the bound values of an instance's mutable columns.
type row struct {
	status, pendingSteps, steps, leaseOwner, lastError any
	data                                               []byte
	updatedAt, deadline, leaseUntil                    int64
}

// encode renders the mutable columns of inst. Pending steps and step records
// are JSON, "" when empty; Data is stored as is.
func (s *Store) encode(inst *saga.Instance) (row, error) {
	pending, err := encodeJSON(inst.PendingSteps)
	if err != nil {
		return row{}, coreerrs.WrapOperation(err, "encode saga pending steps")
	}
	steps := make([]saga.StepRecord, len(inst.Steps))
	for i, st := range inst.Steps {
		st.StartedAt, st.FinishedAt = utc(st.StartedAt), utc(st.FinishedAt)
		steps[i] = st
	}
	stepsJSON, err := encodeJSON(steps)
	if err != nil {
		return row{}, coreerrs.WrapOperation(err, "encode saga steps")
	}
	data := inst.Data
	if data == nil {
		data = []byte{} // NOT NULL column
	}
	d := s.dialect
	return row{
		status:       d.textArg(string(inst.Status)),
		pendingSteps: d.textArg(pending),
		steps:        d.textArg(stepsJSON),
		leaseOwner:   d.textArg(inst.LeaseOwner),
		lastError:    d.textArg(inst.LastError),
		data:         data,
		updatedAt:    unixNano(inst.UpdatedAt),
		deadline:     unixNano(inst.Deadline),
		leaseUntil:   unixNano(inst.LeaseUntil),
	}, nil
}

// scanner is the part of *sql.Row and *sql.Rows scanInstance uses.
type scanner interface {
	Scan(dest ...any) error
}

// scanInstance reads one row selected with columns.
func scanInstance(sc scanner) (*saga.Instance, error) {
	var (
		inst                                     saga.Instance
		status, pending, steps                   string
		stage, created, updated, deadline, lease int64
	)
	if err := sc.Scan(&inst.ID, &inst.Definition, &status, &stage, &pending, &inst.Data, &steps,
		&created, &updated, &deadline, &inst.LeaseOwner, &lease, &inst.Version, &inst.LastError); err != nil {
		return nil, err
	}
	inst.Status = saga.Status(status)
	inst.Stage = int(stage)
	inst.CreatedAt, inst.UpdatedAt = fromUnixNano(created), fromUnixNano(updated)
	inst.Deadline, inst.LeaseUntil = fromUnixNano(deadline), fromUnixNano(lease)
	if pending != "" {
		if err := json.Unmarshal([]byte(pending), &inst.PendingSteps); err != nil {
			return nil, fmt.Errorf("decode saga pending steps: %w", err)
		}
	}
	if steps != "" {
		if err := json.Unmarshal([]byte(steps), &inst.Steps); err != nil {
			return nil, fmt.Errorf("decode saga steps: %w", err)
		}
	}
	return &inst, nil
}

// encodeJSON marshals a slice to JSON, or "" when it is empty.
func encodeJSON[T any](v []T) (string, error) {
	if len(v) == 0 {
		return "", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// unixNano converts a time to Unix nanoseconds, mapping the zero time to 0.
func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// fromUnixNano converts Unix nanoseconds back to a UTC time, mapping 0 to the
// zero time.
func fromUnixNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// utc returns t in UTC, keeping the zero time zero.
func utc(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}
