// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// saveChunk caps the rows of one multi-row INSERT, keeping statements well
// inside every driver's placeholder limit (up to 9 parameters per row).
const saveChunk = 500

// lockChunk caps the IDs one statement locks: the UPDATEs of
// FetchUnprocessedEvents and, on MySQL, of UnlockStuckEvents.
const lockChunk = 1000

// deleteChunk caps the rows one DELETE of DeleteProcessedEvents removes, or
// one UPDATE of ExpireEvents marks, so a large retention or expiry sweep is a series of short transactions rather than one that
// holds its locks, and its undo, for the whole backlog.
const deleteChunk = lockChunk

// Store implements [outbox.Storage] on a SQL database through the standard
// database/sql package. Every time-based predicate — retry backoff, lock
// expiry, retention, expiration — is evaluated against the database clock, and
// the timestamps those predicates compare against are stamped by it too, so
// instances with skewed wall clocks agree on what is due. The one exception is
// an instant the caller sets explicitly ([outbox.Event.CreatedAt],
// [outbox.Event.ExpiresAt]), which is stored as given.
//
// Store is safe for concurrent use. It implements [outbox.Watcher] through
// PostgreSQL LISTEN/NOTIFY when a [Listener] is set (see [Store.Watch]);
// without one [outbox.Outbox.Watch] reports [outbox.ErrWatchUnsupported] and
// dispatch runs on the poll schedule.
type Store struct {
	db                 sqldialect.Pool
	resolveTx          func(context.Context) (*sql.Tx, bool, error)
	dialect            dialect
	table              string // quoted
	name               string // unqualified, for index names
	stmts              statements
	asynchronousCommit bool
	notify             bool     // signal channel on save
	channel            string   // notification channel; empty on MySQL
	listener           Listener // nil: Watch is unsupported
	logger             *slog.Logger
	keyOrder           bool
}

var (
	_ outbox.Storage           = (*Store)(nil)
	_ outbox.BackendClockSaver = (*Store)(nil)
	_ outbox.Watcher           = (*Store)(nil)
)

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	confirmHeads                                                                     string
	fetchPending, fetchFailed, unlock, stuck, deleteProcessed, update, expire, stats string
}

// New creates a Store over db for the given dialect. It performs no I/O; call
// [Store.EnsureSchema] once at startup to create the events table, or apply
// the equivalent DDL through a migration tool.
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName("events_outbox"))
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
	table, err := dl.Table(o.tableName)
	if err != nil {
		return nil, err
	}
	listener := o.listener
	if nilcheck.IsNil(listener) {
		listener = nil
	}

	s := &Store{
		db:                 dl.WrapPool(db),
		resolveTx:          o.txResolver,
		dialect:            dl,
		table:              table,
		name:               sqldialect.Unqualified(o.tableName),
		asynchronousCommit: o.asynchronousCommit && d == DialectPostgres,
		notify:             o.notify,
		listener:           listener,
		logger:             o.logger,
		keyOrder:           o.keyOrder,
	}
	if d != DialectPostgres {
		if s.notify || s.listener != nil {
			return nil, fmt.Errorf("%w: %q", ErrNotifyUnsupported, d)
		}
	} else if s.channel, err = notifyChannel(o.notifyChannel, o.tableName); err != nil && (s.notify || s.listener != nil) {
		return nil, err
	}
	s.stmts = s.buildStatements()
	return s, nil
}

func (s *Store) buildStatements() statements {
	t, d := s.table, s.dialect
	now := d.now
	cols := "id, topic, COALESCE(order_key, topic), payload, attempts, error, " + d.timeCol("created_at") + ", " + d.timeCol("published_at") + ", " +
		d.timeCol("last_attempt_on") + ", " + d.timeCol("expires_at") + ", seq"
	// One statement per status: with the status fixed, the fetch index
	// (status, created_at, seq) returns rows already in ORDER BY order, so each
	// stops at its LIMIT. A single statement over both statuses has to sort
	// the whole backlog first — and on MySQL locks every row it sorted.
	var confirmHeads string
	from := t
	unexpired := " AND (expires_at IS NULL OR expires_at > " + now + ")"
	ready := unexpired
	if s.keyOrder {
		from += " AS candidate"
		const candidateKey = "COALESCE(candidate.order_key, candidate.topic)"
		// Older writers omit order_key. Keep the MySQL indexed column bare.
		sameKey := "(earlier.order_key = " + candidateKey + " OR (earlier.order_key IS NULL AND earlier.topic = " + candidateKey + "))"
		if d.name == DialectPostgres {
			// Match the bounded index expression while comparing the complete binary keys.
			const earlierKey = "COALESCE(earlier.order_key, earlier.topic)"
			sameKey = earlierKey + " = " + candidateKey + " AND substr(" + earlierKey + ", 1, 255) = substr(" + candidateKey + ", 1, 255)"
		}
		blockers := " AND NOT EXISTS (SELECT 1 FROM " + t + " AS earlier WHERE " + sameKey +
			" AND earlier.status IN " + undeliveredStatuses + " AND (earlier.seq < candidate.seq OR earlier.status = 'in-progress'"
		ready += blockers + "))"
		// A competing claim may hold a successor whose status change hasn't
		// committed. Probe visible successors by primary key after locking the
		// candidates. A skipped lock or a newly committed claim blocks the key.
		confirmHeads = "SELECT candidate.id FROM " + from + " WHERE candidate.id IN (%s)" + blockers +
			" OR (earlier.seq > candidate.seq AND COALESCE((SELECT peer.status FROM " + t + " AS peer WHERE peer.id = earlier.id" +
			" FOR UPDATE SKIP LOCKED), 'in-progress') = 'in-progress')))"
	}
	fetch := func(backoff string) string {
		candidates := " FROM " + from + " WHERE status = ?" + backoff + ready + " ORDER BY created_at, seq LIMIT ?"
		if s.keyOrder && d.name == DialectMySQL {
			// A locking status scan can briefly lock a successor it will discard.
			// LIMIT materializes the nonlocking head selection. Drive primary-key
			// locking reads from those IDs, checking readiness on the current row.
			return "SELECT " + cols + " FROM (SELECT id AS head_id, status AS head_status" + candidates +
				") AS heads STRAIGHT_JOIN " + t + " FORCE INDEX (PRIMARY) ON id = heads.head_id" +
				" WHERE status = heads.head_status" + backoff + unexpired + " FOR UPDATE SKIP LOCKED"
		}
		return d.bind("SELECT " + cols + candidates + " FOR UPDATE SKIP LOCKED")
	}
	unlockSet := " SET status = ?, locked_on = NULL, lock_token = NULL, next_attempt_at = NULL"
	stuck := "status = ? AND locked_on < " + d.shifted("-")
	processed := "status IN (?, ?, ?) AND published_at < " + d.shifted("-")
	expiring := "status IN (?, ?) AND expires_at IS NOT NULL AND expires_at <= " + now
	expireSet := " SET status = ?, published_at = " + now + ", locked_on = NULL, lock_token = NULL, next_attempt_at = NULL"
	limit := " LIMIT " + strconv.Itoa(deleteChunk)

	st := statements{
		confirmHeads: confirmHeads,
		fetchPending: fetch(""),
		fetchFailed:  fetch(" AND (next_attempt_at IS NULL OR next_attempt_at <= " + now + ")"),
		// The locking subselect takes the row locks in primary-key order, the
		// order UpdateEvents takes them in, so the two cannot deadlock.
		unlock: d.bind("UPDATE " + t + unlockSet +
			" WHERE id IN (SELECT id FROM " + t + " WHERE " + stuck + " ORDER BY id FOR UPDATE)"),
		// SKIP LOCKED lets concurrent sweeps take disjoint chunks.
		deleteProcessed: d.bind("DELETE FROM " + t +
			" WHERE id IN (SELECT id FROM " + t + " WHERE " + processed + limit + " FOR UPDATE SKIP LOCKED)"),
		update: d.bind("UPDATE " + t + " SET status = ?, attempts = ?, error = ?, last_attempt_on = " + now + ", locked_on = ?," +
			" published_at = CASE WHEN ? THEN " + now + " ELSE NULL END," +
			" next_attempt_at = CASE WHEN ? THEN " + d.shifted("+") + " ELSE NULL END," +
			" lock_token = NULL WHERE id = ? AND lock_token = ?"),
		expire: d.bind("UPDATE " + t + expireSet +
			" WHERE id IN (SELECT id FROM " + t + " WHERE " + expiring + limit + " FOR UPDATE SKIP LOCKED)"),
		stats: d.bind("SELECT" +
			" COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0)," +
			" COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0)," +
			" COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0)," +
			" " + s.oldestAge() +
			" FROM " + t + " WHERE status IN (?, ?, ?, ?, ?)"),
	}
	if d.name == DialectMySQL {
		// MySQL cannot select from the table a statement modifies, and a
		// locking read takes its locks in index-scan order, not ORDER BY order.
		// So the sweep reads a chunk of stuck IDs without locking and updates
		// them through the primary key, which visits — and locks — the rows in
		// key order; the predicate is repeated because the read took no lock.
		st.stuck = "SELECT id FROM " + t + " WHERE " + stuck + " AND id > ? ORDER BY id LIMIT " + strconv.Itoa(lockChunk)
		st.unlock = "UPDATE " + t + " FORCE INDEX (PRIMARY)" + unlockSet + " WHERE id IN (%s) AND " + stuck
		st.deleteProcessed = "DELETE FROM " + t + " WHERE " + processed + limit
		st.expire = "UPDATE " + t + expireSet + " WHERE " + expiring + limit
	}

	return st
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
// With [WithNotify], each INSERT signals in the same statement; a single
// chunk needs no explicit transaction.
//
// An event without a CreatedAt is stamped with the database clock; see
// [Store.SaveEventsWithTTL] for the expiry counterpart.
func (s *Store) SaveEvents(ctx context.Context, events ...outbox.Event) error {
	return s.SaveEventsWithTTL(ctx, 0, events...)
}

// SaveEventsWithTTL implements [outbox.BackendClockSaver]: it inserts like
// [Store.SaveEvents] and stamps what the caller left unset from the database
// clock — the clock every later predicate compares against. An event without a
// CreatedAt is created "now"; one without an ExpiresAt expires ttl after its
// creation when ttl is positive. Instants the caller did set are stored as
// given.
func (s *Store) SaveEventsWithTTL(ctx context.Context, ttl time.Duration, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return s.insert(ctx, tx, ttl, events)
	}
	if s.resolveTx != nil {
		tx, open, err := s.resolveTx(ctx)
		if err != nil {
			return err
		}
		if tx != nil {
			if !open {
				return sql.ErrTxDone
			}
			return s.insert(ctx, tx, ttl, events)
		}
	}
	if len(events) <= saveChunk {
		return s.insert(ctx, s.db, ttl, events)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreerrs.WrapOperation(err, "begin outbox save transaction")
	}
	if err := s.insert(ctx, tx, ttl, events); err != nil {
		_ = tx.Rollback()
		return err
	}
	return coreerrs.WrapOperation(tx.Commit(), "commit outbox save transaction")
}

func (s *Store) insert(ctx context.Context, ex execer, ttl time.Duration, events []outbox.Event) error {
	d := s.dialect
	for start := 0; start < len(events); start += saveChunk {
		chunk := events[start:min(start+saveChunk, len(events))]
		args := make([]any, 0, len(chunk)*9) //nolint:mnd // columns per row
		var rows strings.Builder
		for i, ev := range chunk {
			if i > 0 {
				rows.WriteString(", ")
			}
			rows.WriteString("(?, ?, ?, ?, ?, ")
			args = append(args, d.textArg(ev.Id), []byte(ev.Key), []byte(cmp.Or(ev.OrderKey, ev.Key)), ev.Payload, string(ev.Status))

			// A zero CreatedAt is stamped by the database; so is the expiry
			// derived from it. An explicit CreatedAt anchors the default expiry
			// instead, which needs no clock at all.
			expiresAt := ev.ExpiresAt
			if ev.CreatedAt.IsZero() {
				rows.WriteString(d.now)
			} else {
				rows.WriteString("?")
				args = append(args, d.timeArg(ev.CreatedAt))
				if expiresAt.IsZero() && ttl > 0 {
					expiresAt = ev.CreatedAt.Add(ttl)
				}
			}

			rows.WriteString(", ?, ?, ")
			args = append(args, int64(ev.Attempts), nullableBytes(ev.LastError))

			if expiresAt.IsZero() && ttl > 0 {
				rows.WriteString(d.shifted("+"))
				args = append(args, d.duration(ttl))
			} else {
				rows.WriteString("?")
				args = append(args, d.timeArg(expiresAt))
			}
			rows.WriteString(")")
		}
		query := "INSERT INTO " + s.table + " (id, topic, order_key, payload, status, created_at, attempts, error, expires_at) VALUES " + rows.String()
		if s.notify {
			query = "WITH ins AS (" + query + " RETURNING 1) SELECT pg_notify(?, '') FROM ins LIMIT 1"
			args = append(args, s.channel)
		}
		query = d.bind(query)
		if _, err := ex.ExecContext(ctx, query, args...); err != nil {
			return coreerrs.WrapOperation(err, "insert outbox events")
		}
	}
	return nil
}

// dispatchTx retries the entire transaction when the database aborts it.
func dispatchTx[T any](ctx context.Context, s *Store, opts *sql.TxOptions, run func(*sql.Tx) (T, error)) (T, error) {
	var result T
	err := deadlockRetry.Do(ctx, func(ctx context.Context) error {
		tx, err := s.db.BeginTx(ctx, opts)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if s.asynchronousCommit {
			if _, err = tx.ExecContext(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
				return coreerrs.WrapOperation(err, "set outbox transaction durability")
			}
		}
		value, err := run(tx)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		result = value
		return nil
	})
	// database/sql may finish rolling back a canceled transaction before a
	// statement or Commit can report the context error.
	if err != nil && ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, err
}

func (s *Store) execDispatch(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !s.asynchronousCommit {
		var result sql.Result
		err := deadlockRetry.Do(ctx, func(ctx context.Context) error {
			var err error
			result, err = s.db.ExecContext(ctx, query, args...)
			return err
		})
		return result, err
	}
	return dispatchTx(ctx, s, nil, func(tx *sql.Tx) (sql.Result, error) {
		return tx.ExecContext(ctx, query, args...)
	})
}

// FetchUnprocessedEvents locks and returns up to batchSize events that are
// ready to dispatch — pending ones, plus failed ones whose backoff elapsed —
// oldest first, events saved together in insertion order. The rows are
// selected FOR UPDATE SKIP LOCKED and marked in-progress in one transaction,
// so concurrent dispatchers take disjoint batches without blocking each other.
// With [WithKeyOrder], an earlier undelivered sequence blocks the same key
// even during retry backoff. An in-progress event blocks its key at any sequence.
// Every returned event carries the lock token this call minted.
func (s *Store) FetchUnprocessedEvents(ctx context.Context, batchSize uint32) ([]outbox.Event, error) {
	if batchSize == 0 {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "outbox fetch batch size must be positive")
	}
	events, err := dispatchTx(ctx, s, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(tx *sql.Tx) ([]outbox.Event, error) {
		return s.claim(ctx, tx, batchSize)
	})
	return events, coreerrs.WrapOperation(err, "claim outbox events")
}

func (s *Store) claim(ctx context.Context, tx *sql.Tx, batchSize uint32) ([]outbox.Event, error) {
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

// readyEvent is a fetched event with its insertion sequence, which orders
// events sharing a CreatedAt.
type readyEvent struct {
	outbox.Event
	seq int64
}

// selectReady locks up to batchSize pending and up to batchSize due failed
// events, each oldest first, and returns the oldest batchSize of both. The
// surplus rows stay locked only until the fetch transaction ends.
func (s *Store) selectReady(ctx context.Context, tx *sql.Tx, batchSize uint32) ([]outbox.Event, error) {
	var ready []readyEvent
	for _, q := range [...]struct {
		stmt   string
		status outbox.Status
	}{
		{s.stmts.fetchPending, outbox.StatusPending},
		{s.stmts.fetchFailed, outbox.StatusFailed},
	} {
		var err error
		if ready, err = s.appendReady(ctx, tx, ready, q.stmt, string(q.status), int64(batchSize)); err != nil {
			return nil, err
		}
	}

	if s.keyOrder {
		// The candidate locks are held now. A fresh read also sees a lower
		// sequence that committed between the pending and failed reads.
		heads := ready[:0]
		for chunk := range slices.Chunk(ready, lockChunk) {
			confirmed, err := s.confirmHeads(ctx, tx, chunk)
			if err != nil {
				return nil, err
			}
			heads = append(heads, confirmed...)
		}
		ready = heads
	}

	slices.SortFunc(ready, func(a, b readyEvent) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.seq, b.seq))
	})
	events := make([]outbox.Event, min(len(ready), int(batchSize)))
	for i := range events {
		events[i] = ready[i].Event
	}
	return events, nil
}

func (s *Store) confirmHeads(ctx context.Context, tx *sql.Tx, candidates []readyEvent) ([]readyEvent, error) {
	ids := make([]any, 0, len(candidates))
	for _, ev := range candidates {
		ids = append(ids, s.dialect.textArg(ev.Id))
	}
	query := s.dialect.bind(sqldialect.InList(s.stmts.confirmHeads, len(ids)))
	rows, err := tx.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	heads := make(map[string]bool, len(candidates))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		heads[id] = true
	}
	return slices.DeleteFunc(candidates, func(ev readyEvent) bool { return !heads[ev.Id] }), rows.Err()
}

func (s *Store) appendReady(ctx context.Context, tx *sql.Tx, ready []readyEvent, stmt string, args ...any) ([]readyEvent, error) {
	rows, err := tx.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "select outbox events")
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		ev, err := s.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		ready = append(ready, ev)
	}
	return ready, coreerrs.WrapOperation(rows.Err(), "read outbox events")
}

func (s *Store) scanEvent(rows *sql.Rows) (readyEvent, error) {
	var (
		ev       readyEvent
		id       []byte
		topic    []byte
		orderKey []byte
		attempts int64
		lastErr  []byte
	)
	var ts [4]timeScan // created_at, published_at, last_attempt_on, expires_at
	d := s.dialect
	if err := rows.Scan(&id, &topic, &orderKey, &ev.Payload, &attempts, &lastErr,
		d.timeDest(&ts[0]), d.timeDest(&ts[1]), d.timeDest(&ts[2]), d.timeDest(&ts[3]), &ev.seq); err != nil {
		return readyEvent{}, coreerrs.WrapOperation(err, "scan outbox event")
	}
	ev.Id, ev.Key, ev.OrderKey = string(id), string(topic), string(orderKey)
	ev.Attempts = uint32(attempts) //nolint:gosec // stored from a uint32
	if lastErr != nil {
		msg := string(lastErr)
		ev.LastError = &msg
	}
	var err error
	for _, f := range [...]struct {
		dst *time.Time
		src *timeScan
	}{
		{&ev.CreatedAt, &ts[0]},
		{&ev.PublishedAt, &ts[1]},
		{&ev.LastAttemptOn, &ts[2]},
		{&ev.ExpiresAt, &ts[3]},
	} {
		if *f.dst, err = d.timeValue(f.src); err != nil {
			return readyEvent{}, coreerrs.WrapOperation(err, "parse outbox event timestamp")
		}
	}
	return ev, nil
}

// UpdateEvents writes each event's processing outcome, fenced by its lock
// token: a write whose token no longer matches — the sweeper reclaimed the
// event and another dispatcher took it — affects no row and is dropped. The
// attempt time, the completion time and the retry deadline (now + RetryAfter)
// all come from the database clock. All writes share one transaction, which
// takes its row locks in primary-key order — the order the unlock sweep takes
// them in — and is retried if the database still reports a deadlock.
func (s *Store) UpdateEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	ordered := slices.SortedFunc(slices.Values(events), func(a, b outbox.Event) int { return cmp.Compare(a.Id, b.Id) })
	_, err := dispatchTx(ctx, s, nil, func(tx *sql.Tx) (struct{}, error) {
		return struct{}{}, s.updateEvents(ctx, tx, ordered)
	})
	return coreerrs.WrapOperation(err, "update outbox events")
}

func (s *Store) updateEvents(ctx context.Context, tx *sql.Tx, events []outbox.Event) error {
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
	return nil
}

// UnlockStuckEvents returns in-progress events locked longer than lockExpiry
// (by the database clock) to pending, clearing the lock token so the original
// holder's late write is fenced out.
//
// The sweep takes its row locks in primary-key order — the order
// [Store.UpdateEvents] takes them in — and is retried if the database still
// reports a deadlock.
func (s *Store) UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error {
	unlock := s.unlockStuck
	if s.dialect.name == DialectMySQL {
		unlock = s.unlockStuckByID
	}
	err := unlock(ctx, string(outbox.StatusPending), string(outbox.StatusInProgress), s.dialect.duration(lockExpiry))
	return coreerrs.WrapOperation(err, "unlock stuck outbox events")
}

func (s *Store) unlockStuck(ctx context.Context, pending, inProgress string, age any) error {
	_, err := s.execDispatch(ctx, s.stmts.unlock, pending, inProgress, age)
	return err
}

// unlockStuckByID is the MySQL sweep: chunks of stuck IDs, read in key order
// without locking, each returned to pending through the primary key.
func (s *Store) unlockStuckByID(ctx context.Context, pending, inProgress string, age any) error {
	after := []byte{}
	for {
		ids, err := dispatchTx(ctx, s, nil, func(tx *sql.Tx) ([]any, error) {
			ids, err := s.stuckIDs(ctx, tx, inProgress, age, after)
			if err != nil || len(ids) == 0 {
				return nil, err
			}
			args := make([]any, 0, len(ids)+3) //nolint:mnd // both statuses and the lock age
			args = append(append(append(args, pending), ids...), inProgress, age)
			query := fmt.Sprintf(s.stmts.unlock, "?"+strings.Repeat(", ?", len(ids)-1))
			_, err = tx.ExecContext(ctx, query, args...)
			return ids, err
		})
		if err != nil {
			return err
		}
		if len(ids) < lockChunk {
			return nil
		}
		after, _ = ids[len(ids)-1].([]byte)
	}
}

func (s *Store) stuckIDs(ctx context.Context, tx *sql.Tx, inProgress string, age any, after []byte) ([]any, error) {
	rows, err := tx.QueryContext(ctx, s.stmts.stuck, inProgress, age, after)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []any
	for rows.Next() {
		var id []byte
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteProcessedEvents removes sent, skipped and expired events published
// before now minus olderThan (database clock). Dead letters are retained.
//
// Rows are deleted [deleteChunk] at a time, each chunk in its own short
// transaction, until a chunk comes back short; ctx is checked between chunks.
// A sweep cut short keeps what it already deleted.
func (s *Store) DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error {
	for {
		res, err := s.execDispatch(ctx, s.stmts.deleteProcessed,
			string(outbox.StatusSent), string(outbox.StatusSkipped), string(outbox.StatusExpired), s.dialect.duration(olderThan))
		if err != nil {
			return coreerrs.WrapOperation(err, "delete processed outbox events")
		}
		n, err := res.RowsAffected()
		if err != nil {
			return coreerrs.WrapOperation(err, "delete processed outbox events")
		}
		if n < deleteChunk {
			return nil
		}
	}
}

// ExpireEvents marks pending and failed events whose ExpiresAt has passed
// (database clock) as expired and stamps their completion time for retention.
//
// Like [Store.DeleteProcessedEvents] it works [deleteChunk] rows at a time,
// each chunk its own short transaction, until a chunk comes back short, so an
// expired backlog after an outage is not one long transaction that locks it
// all. It returns the events expired, including chunks before an error.
func (s *Store) ExpireEvents(ctx context.Context) (int64, error) {
	var total int64
	for {
		res, err := s.execDispatch(ctx, s.stmts.expire,
			string(outbox.StatusExpired), string(outbox.StatusPending), string(outbox.StatusFailed))
		if err != nil {
			return total, coreerrs.WrapOperation(err, "expire outbox events")
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, coreerrs.WrapOperation(err, "expire outbox events")
		}
		total += n
		if n < deleteChunk {
			return total, nil
		}
	}
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
