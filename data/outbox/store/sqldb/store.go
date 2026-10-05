// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/data/outbox"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// saveChunk caps the rows of one multi-row INSERT, keeping statements well
// inside every driver's placeholder limit (8 columns per row).
const saveChunk = 500

// lockChunk caps the IDs locked by one UPDATE in FetchUnprocessedEvents.
const lockChunk = 1000

// Store implements [outbox.Store] on a SQL database through the standard
// database/sql package. Every time-based predicate — retry backoff, lock
// expiry, retention, expiration — is evaluated against the database clock, and
// the timestamps those predicates compare against are stamped by it too, so
// instances with skewed wall clocks agree on what is due.
//
// Store is safe for concurrent use. It does not implement [outbox.Watcher]:
// database/sql has no notification API, so dispatch runs on the poll schedule
// and [outbox.Outbox.Watch] reports [outbox.ErrWatchUnsupported].
type Store struct {
	db            *sql.DB
	dialect       dialect
	table         string // quoted
	name          string // unqualified, for index names
	schemaTimeout time.Duration
	stmts         statements
}

var _ outbox.Store = (*Store)(nil)

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	fetch, unlock, deleteProcessed, update, expire, stats string
}

// New creates a Store over db and, like the MongoDB store creating its
// indexes, creates the events table and indexes if they do not exist (bounded
// by [WithSchemaCreateTimeout], under the [WithContext] base context).
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName("events_outbox"))
func New(db *sql.DB, d Dialect, opts ...Option) (*Store, error) {
	if db == nil {
		return nil, errors.New("sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)
	table, err := dl.table(o.tableName)
	if err != nil {
		return nil, err
	}

	s := &Store{
		db:            db,
		dialect:       dl,
		table:         table,
		name:          o.tableName[strings.LastIndexByte(o.tableName, '.')+1:],
		schemaTimeout: o.schemaTimeout,
	}
	s.stmts = s.buildStatements()
	if err := s.createSchema(corecontext.OrBackground(o.ctx)); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) buildStatements() statements {
	t, d := s.table, s.dialect
	now := d.now
	cols := "id, topic, payload, attempts, error, " + d.timeCol("created_at") + ", " + d.timeCol("published_at") + ", " +
		d.timeCol("last_attempt_on") + ", " + d.timeCol("expires_at")
	return statements{
		fetch: d.bind("SELECT " + cols + " FROM " + t +
			" WHERE (status = ? OR (status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= " + now + ")))" +
			" AND (expires_at IS NULL OR expires_at > " + now + ")" +
			" ORDER BY created_at, seq LIMIT ? FOR UPDATE SKIP LOCKED"),
		unlock: d.bind("UPDATE " + t + " SET status = ?, locked_on = NULL, lock_token = NULL, next_attempt_at = NULL" +
			" WHERE status = ? AND locked_on < " + d.shifted("-")),
		deleteProcessed: d.bind("DELETE FROM " + t + " WHERE status IN (?, ?, ?) AND published_at < " + d.shifted("-")),
		update: d.bind("UPDATE " + t + " SET status = ?, attempts = ?, error = ?, last_attempt_on = " + now + ", locked_on = ?," +
			" published_at = CASE WHEN ? THEN " + now + " ELSE NULL END," +
			" next_attempt_at = CASE WHEN ? THEN " + d.shifted("+") + " ELSE NULL END," +
			" lock_token = NULL WHERE id = ? AND lock_token = ?"),
		expire: d.bind("UPDATE " + t + " SET status = ?, published_at = " + now + ", locked_on = NULL, lock_token = NULL, next_attempt_at = NULL" +
			" WHERE status IN (?, ?) AND expires_at IS NOT NULL AND expires_at <= " + now),
		stats: d.bind("SELECT" +
			" COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0)," +
			" COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0)," +
			" COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0)," +
			" " + s.oldestAge() +
			" FROM " + t + " WHERE status IN (?, ?, ?, ?, ?)"),
	}
}

// oldestAge renders the age in milliseconds of the oldest waiting event,
// measured against the database clock; 0 when nothing waits.
func (s *Store) oldestAge() string {
	oldest := "MIN(CASE WHEN status IN (?, ?) THEN created_at END)"
	if s.dialect.name == DialectPostgres {
		return "COALESCE(CAST(EXTRACT(EPOCH FROM (now() - " + oldest + ")) * 1000 AS BIGINT), 0)"
	}
	return "COALESCE(TIMESTAMPDIFF(MICROSECOND, " + oldest + ", UTC_TIMESTAMP(6)) DIV 1000, 0)"
}

// txKey carries a caller transaction for [WithTx].
type txKey struct{}

// WithTx returns a context carrying tx. [Store.SaveEvents] runs on that
// transaction, so [outbox.Outbox.Save] joins the caller's business transaction
// — the atomicity the outbox exists for. Other Store methods ignore it.
//
//	tx, err := db.BeginTx(ctx, nil)
//	// ... business writes on tx ...
//	if err := ob.Save(sqldb.WithTx(ctx, tx), outbox.Event{Key: "orders.created", Payload: data}); err != nil {
//		_ = tx.Rollback()
//		return err
//	}
//	return tx.Commit()
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// execer is the part of *sql.DB and *sql.Tx SaveEvents uses.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// SaveEvents inserts new events on the transaction carried by ctx (see
// [WithTx]), or on the database otherwise. A batch larger than one INSERT is
// still all-or-nothing: without a caller transaction it runs in its own.
func (s *Store) SaveEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return s.insert(ctx, tx, events)
	}
	if len(events) <= saveChunk {
		return s.insert(ctx, s.db, events)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreerrs.WrapOperation(err, "begin outbox save transaction")
	}
	if err := s.insert(ctx, tx, events); err != nil {
		_ = tx.Rollback()
		return err
	}
	return coreerrs.WrapOperation(tx.Commit(), "commit outbox save transaction")
}

func (s *Store) insert(ctx context.Context, ex execer, events []outbox.Event) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?)"
	for start := 0; start < len(events); start += saveChunk {
		chunk := events[start:min(start+saveChunk, len(events))]
		args := make([]any, 0, len(chunk)*8) //nolint:mnd // columns per row
		for _, ev := range chunk {
			args = append(args, s.dialect.textArg(ev.Id), []byte(ev.Key), ev.Payload, string(ev.Status),
				s.dialect.timeArg(ev.CreatedAt), int64(ev.Attempts), nullableBytes(ev.LastError), s.dialect.timeArg(ev.ExpiresAt))
		}
		query := s.dialect.bind("INSERT INTO " + s.table + " (id, topic, payload, status, created_at, attempts, error, expires_at) VALUES " +
			row + strings.Repeat(", "+row, len(chunk)-1))
		if _, err := ex.ExecContext(ctx, query, args...); err != nil {
			return coreerrs.WrapOperation(err, "insert outbox events")
		}
	}
	return nil
}

// FetchUnprocessedEvents locks and returns up to batchSize events that are
// ready to dispatch — pending ones, plus failed ones whose backoff elapsed —
// oldest first, events saved together in insertion order. The rows are
// selected FOR UPDATE SKIP LOCKED and marked in-progress in one transaction,
// so concurrent dispatchers take disjoint batches without blocking each other.
// Every returned event carries the lock token this call minted.
func (s *Store) FetchUnprocessedEvents(ctx context.Context, batchSize uint32) ([]outbox.Event, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "begin outbox fetch transaction")
	}
	defer func() { _ = tx.Rollback() }()

	events, err := s.selectReady(ctx, tx, batchSize)
	if err != nil || len(events) == 0 {
		return events, err
	}

	token := uuid.New().String()
	// One UPDATE per chunk keeps each statement far below driver parameter
	// limits (pgx: 65535) however large the configured batch is.
	for start := 0; start < len(events); start += lockChunk {
		chunk := events[start:min(start+lockChunk, len(events))]
		args := make([]any, 0, len(chunk)+2) //nolint:mnd // status and token
		args = append(args, string(outbox.StatusInProgress), token)
		for _, ev := range chunk {
			args = append(args, s.dialect.textArg(ev.Id))
		}
		lock := s.dialect.bind("UPDATE " + s.table + " SET status = ?, locked_on = " + s.dialect.now + ", lock_token = ? WHERE id IN (?" +
			strings.Repeat(", ?", len(chunk)-1) + ")")
		if _, err := tx.ExecContext(ctx, lock, args...); err != nil {
			return nil, coreerrs.WrapOperation(err, "lock outbox events")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, coreerrs.WrapOperation(err, "commit outbox fetch transaction")
	}

	// LockedOn is informational (the dispatcher clears it); the authoritative
	// lock time is the database clock written above.
	lockedAt := time.Now().UTC()
	for i := range events {
		events[i].Status = outbox.StatusInProgress
		events[i].LockToken = token
		events[i].LockedOn = lockedAt
	}
	return events, nil
}

func (s *Store) selectReady(ctx context.Context, tx *sql.Tx, batchSize uint32) ([]outbox.Event, error) {
	rows, err := tx.QueryContext(ctx, s.stmts.fetch, string(outbox.StatusPending), string(outbox.StatusFailed), int64(batchSize))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "select outbox events")
	}
	defer func() { _ = rows.Close() }()

	events := []outbox.Event{}
	for rows.Next() {
		ev, err := s.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, coreerrs.WrapOperation(rows.Err(), "read outbox events")
}

func (s *Store) scanEvent(rows *sql.Rows) (outbox.Event, error) {
	var (
		ev       outbox.Event
		id       []byte
		topic    []byte
		attempts int64
		lastErr  []byte
	)
	var ts [4]timeScan // created_at, published_at, last_attempt_on, expires_at
	d := s.dialect
	if err := rows.Scan(&id, &topic, &ev.Payload, &attempts, &lastErr,
		d.timeDest(&ts[0]), d.timeDest(&ts[1]), d.timeDest(&ts[2]), d.timeDest(&ts[3])); err != nil {
		return outbox.Event{}, coreerrs.WrapOperation(err, "scan outbox event")
	}
	ev.Id, ev.Key = string(id), string(topic)
	ev.Attempts = uint32(attempts) //nolint:gosec // stored from a uint32
	if lastErr != nil {
		msg := string(lastErr)
		ev.LastError = &msg
	}
	var err error
	for i, dst := range [...]*time.Time{&ev.CreatedAt, &ev.PublishedAt, &ev.LastAttemptOn, &ev.ExpiresAt} {
		if *dst, err = d.timeValue(&ts[i]); err != nil {
			return outbox.Event{}, coreerrs.WrapOperation(err, "parse outbox event timestamp")
		}
	}
	return ev, nil
}

// UpdateEvents writes each event's processing outcome, fenced by its lock
// token: a write whose token no longer matches — the sweeper reclaimed the
// event and another dispatcher took it — affects no row and is dropped. The
// attempt time, the completion time and the retry deadline (now + RetryAfter)
// all come from the database clock. All writes share one transaction.
func (s *Store) UpdateEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreerrs.WrapOperation(err, "begin outbox update transaction")
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, s.stmts.update)
	if err != nil {
		return coreerrs.WrapOperation(err, "prepare outbox update")
	}
	defer func() { _ = stmt.Close() }()

	for _, ev := range events {
		if _, err := stmt.ExecContext(ctx,
			string(ev.Status), int64(ev.Attempts), nullableBytes(ev.LastError), s.dialect.timeArg(ev.LockedOn),
			!ev.PublishedAt.IsZero(),
			ev.RetryAfter > 0, s.dialect.duration(max(ev.RetryAfter, 0)),
			s.dialect.textArg(ev.Id), ev.LockToken,
		); err != nil {
			return coreerrs.WrapOperation(err, "update outbox event")
		}
	}
	return coreerrs.WrapOperation(tx.Commit(), "commit outbox update transaction")
}

// UnlockStuckEvents returns in-progress events locked longer than lockExpiry
// (by the database clock) to pending, clearing the lock token so the original
// holder's late write is fenced out.
func (s *Store) UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error {
	if _, err := s.db.ExecContext(ctx, s.stmts.unlock,
		string(outbox.StatusPending), string(outbox.StatusInProgress), s.dialect.duration(lockExpiry)); err != nil {
		return coreerrs.WrapOperation(err, "unlock stuck outbox events")
	}
	return nil
}

// DeleteProcessedEvents removes sent, skipped and expired events published
// before now minus olderThan (database clock). Dead letters are retained.
func (s *Store) DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error {
	if _, err := s.db.ExecContext(ctx, s.stmts.deleteProcessed,
		string(outbox.StatusSent), string(outbox.StatusSkipped), string(outbox.StatusExpired), s.dialect.duration(olderThan)); err != nil {
		return coreerrs.WrapOperation(err, "delete processed outbox events")
	}
	return nil
}

// ExpireEvents marks pending and failed events whose ExpiresAt has passed
// (database clock) as expired and stamps their completion time for retention.
func (s *Store) ExpireEvents(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.stmts.expire,
		string(outbox.StatusExpired), string(outbox.StatusPending), string(outbox.StatusFailed))
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "expire outbox events")
	}
	n, err := res.RowsAffected()
	return n, coreerrs.WrapOperation(err, "expire outbox events")
}

// Stats returns the backlog snapshot in one aggregate query; the age of the
// oldest waiting event is measured against the database clock.
func (s *Store) Stats(ctx context.Context) (outbox.Stats, error) {
	pending, failed := string(outbox.StatusPending), string(outbox.StatusFailed)
	inProgress := string(outbox.StatusInProgress)
	maxAttempts, rejected := string(outbox.StatusMaxAttemptReached), string(outbox.StatusRejected)

	var st outbox.Stats
	var ageMs int64
	err := s.db.QueryRowContext(ctx, s.stmts.stats,
		pending, failed, inProgress, maxAttempts, rejected, pending, failed,
		inProgress, pending, failed, maxAttempts, rejected,
	).Scan(&st.Pending, &st.InProgress, &st.DeadLettered, &ageMs)
	if err != nil {
		return outbox.Stats{}, coreerrs.WrapOperation(err, "query outbox stats")
	}
	st.OldestPendingAge = time.Duration(max(ageMs, 0)) * time.Millisecond
	return st, nil
}
