// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// columns is the column list of the items table in scan order.
const columns = "id, generation, state, since, attempts, next_attempt_at, expired_at, resolved_at, flagged, flag_reason, payload"

// markAttempts bounds the insert-then-read rounds of Mark: the read misses the
// item only when it was deleted between the two, and the next round inserts
// it again.
const markAttempts = 2

// reopenAttempts bounds the update-then-mark rounds of Reopen when a writer
// inserts an older or closed item between the two statements.
const reopenAttempts = 2

// keyChunk caps the keys one Requeue statement lists, keeping statements well
// inside every driver's placeholder limit.
const keyChunk = 1000

// State literals. The states are constants of this package's own, so they are
// written into the statements rather than bound.
var (
	pending  = "'" + string(retryqueue.StatePending) + "'"
	expired  = "'" + string(retryqueue.StateExpired) + "'"
	resolved = "'" + string(retryqueue.StateResolved) + "'"
)

var (
	_ retryqueue.AttemptStorage[struct{}] = (*Store[struct{}])(nil)
	_ retryqueue.Mover                    = (*Store[struct{}])(nil)
)

// Store is the SQL-backed [retryqueue.Storage] and [retryqueue.Mover]: one row
// per item keyed by its key, the payload P encoded by the configured codec.
// Every transition is one conditional statement, so the database row lock
// serializes racing callers and exactly one of them makes a transition.
//
// Instants are the caller's, stored as Unix nanoseconds at full precision;
// only [Store.PurgeExpired] reads the database clock. Every method runs on
// the transaction ctx carries ([WithTx]), or on the database otherwise. Store
// is safe for concurrent use. With [WithPurgeInterval] it also purges resolved
// items in the background from its first write call until [Store.Close].
type Store[P any] struct {
	db             sqldialect.Pool
	dialect        dialect
	table          string // quoted
	tableName      string // unqualified, for index names and the DDL lock
	codec          serializer.Serializer
	retention      time.Duration
	purgeBatchSize int
	stmts          statements

	purge *purgeloop.Loop
}

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	get, getMark, exists, insert, reopen                string
	listDue, wakeDue, expire                            string
	reschedule, resolve, flag, unflag                   fencedQuery
	deleteObsolete, newer, remove, listByState, requeue string
	move, purge                                         string
}

type fencedQuery struct {
	generation string
	attempt    string
}

// New creates a Store over db for the given dialect. It performs no I/O; call
// [Store.EnsureSchema] once at startup to create the table, or apply the
// equivalent DDL through a migration tool.
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	queue, err := sqldb.New[Valuation](db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := queue.EnsureSchema(ctx); err != nil {
//		return err
//	}
func New[P any](db *sql.DB, d Dialect, opts ...Option) (*Store[P], error) {
	if db == nil {
		return nil, errors.New("retryqueue sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)
	s := &Store[P]{
		db:             dl.WrapPool(db).WithResolver(o.resolveTx),
		dialect:        dl,
		tableName:      sqldialect.Unqualified(o.tableName),
		codec:          o.codec,
		retention:      o.resolvedRetention,
		purgeBatchSize: o.purgeBatchSize,
	}
	every := o.purgeInterval
	if o.resolvedRetention <= 0 {
		every = 0
	}
	s.purge = purgeloop.New(every, s.PurgeExpired, o.logger, "resolved retry items")
	if s.table, err = dl.Table(o.tableName); err != nil {
		return nil, err
	}
	s.stmts = s.buildStatements()
	return s, nil
}

func (s *Store[P]) buildStatements() statements {
	t, b := s.table, s.dialect.bind
	fenced := func(query string) fencedQuery {
		return fencedQuery{generation: b(query), attempt: b(query + " AND since = ?")}
	}
	sel := "SELECT " + columns + " FROM " + t
	insert := "INSERT INTO " + t + " (" + columns + ") VALUES (?, ?, " + pending + ", ?, 0, ?, 0, 0, FALSE, '', ?)"
	// The read-back of Mark follows an insert that found the key taken. A
	// locking read on MySQL sees the latest committed row even inside a caller
	// transaction whose snapshot predates it. PostgreSQL reads it under READ
	// COMMITTED; at REPEATABLE READ or SERIALIZABLE, an insert committed after
	// the caller's snapshot causes a serialization error (SQLSTATE 40001).
	getMark := sel + " WHERE id = ?"
	// DeleteObsolete's lookup follows a DELETE that read the current row; it
	// must read the same row, not an older snapshot, for the same reason.
	newer := "SELECT 1 FROM " + t + " WHERE id = ? AND generation > ?"
	if s.dialect.name == DialectPostgres {
		insert += " ON CONFLICT (id) DO NOTHING"
	} else {
		getMark += " LOCK IN SHARE MODE"
		newer += " LOCK IN SHARE MODE"
	}
	purge := "DELETE FROM " + t + " WHERE id IN (SELECT id FROM " + t + " WHERE state = " + resolved +
		" AND resolved_at <> 0 AND resolved_at <= " + s.dialect.nowNanos + " - ? LIMIT " + strconv.Itoa(s.purgeBatchSize) +
		" FOR UPDATE SKIP LOCKED)"
	if s.dialect.name == DialectMySQL {
		// MySQL cannot select from the table a statement modifies.
		purge = "DELETE FROM " + t + " WHERE state = " + resolved + " AND resolved_at <> 0 AND resolved_at <= " +
			s.dialect.nowNanos + " - ? ORDER BY resolved_at LIMIT " + strconv.Itoa(s.purgeBatchSize)
	}
	return statements{
		get:     b(sel + " WHERE id = ?"),
		getMark: b(getMark),
		exists:  b("SELECT 1 FROM " + t + " WHERE id = ?"),
		insert:  b(insert),
		reopen: b("UPDATE " + t + " SET generation = ?, state = " + pending + ", since = ?, attempts = 0, next_attempt_at = ?," +
			" expired_at = 0, resolved_at = 0, flagged = FALSE, flag_reason = '', payload = ?" +
			" WHERE id = ? AND (generation < ? OR (generation = ? AND since < ? AND state <> " + pending + "))"),
		listDue: b(sel + " WHERE state = " + pending + " AND next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?"),
		wakeDue: b("UPDATE " + t + " SET next_attempt_at = ? WHERE state = " + pending + " AND next_attempt_at > ?"),
		reschedule: fenced("UPDATE " + t + " SET attempts = attempts + 1, next_attempt_at = ?" +
			" WHERE id = ? AND state = " + pending + " AND generation = ?"),
		expire: b("UPDATE " + t + " SET state = " + expired + ", expired_at = ?" +
			" WHERE id = ? AND state = " + pending + " AND generation = ? AND since = ?"),
		resolve: fenced("UPDATE " + t + " SET state = " + resolved + ", resolved_at = ?, expired_at = 0, flagged = FALSE, flag_reason = ''" +
			" WHERE id = ? AND state <> " + resolved + " AND generation = ?"),
		flag: fenced("UPDATE " + t + " SET flagged = TRUE, flag_reason = ?" +
			" WHERE id = ? AND state = " + pending + " AND generation = ? AND (flagged = FALSE OR flag_reason <> ?)"),
		unflag:         fenced("UPDATE " + t + " SET flagged = FALSE, flag_reason = '' WHERE id = ? AND generation = ? AND flagged = TRUE"),
		deleteObsolete: b("DELETE FROM " + t + " WHERE id = ? AND generation <= ?"),
		newer:          b(newer),
		remove:         b("DELETE FROM " + t + " WHERE id = ?"),
		listByState:    b(sel + " WHERE state = ? ORDER BY since, id LIMIT ?"),
		// Requeue appends its key list, then binds; see [Store.Requeue].
		requeue: "UPDATE " + t + " SET state = " + pending + ", since = GREATEST(since + 1, ?), next_attempt_at = ?, attempts = 0, expired_at = 0" +
			" WHERE state = " + expired,
		move:  b("UPDATE " + t + " SET id = ? WHERE id = ?"),
		purge: b(purge),
	}
}

// txKey carries a caller transaction for [WithTx].
type txKey struct{}

// WithTx returns a context carrying tx. Every Store method called with it runs
// on that transaction, so the queue's writes commit or roll back with the
// caller's other writes — the counterpart of a MongoDB session context.
//
//	tx, err := db.BeginTx(ctx, nil)
//	// ... business writes on tx ...
//	if err := queue.Move(sqldb.WithTx(ctx, tx), oldKey, newKey); err != nil {
//		_ = tx.Rollback()
//		return err
//	}
//	return tx.Commit()
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func (o *options) resolveTx(ctx context.Context) (*sql.Tx, bool) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return tx, true
	}
	if o.txResolver != nil {
		return o.txResolver(ctx)
	}
	return nil, false
}

// Mark implements [retryqueue.Storage] with an insert-if-absent — ON CONFLICT
// DO NOTHING on PostgreSQL, a plain INSERT whose duplicate-key error is
// absorbed on MySQL — and, when the key was taken, a read of the stored item.
// Of concurrent first marks the database lets exactly one insert. Inside a
// caller transaction on PostgreSQL at READ COMMITTED a taken key leaves the
// transaction usable. At REPEATABLE READ or SERIALIZABLE, if the key was
// inserted by a transaction that committed after the caller's snapshot, Mark
// fails with a serialization error (SQLSTATE 40001); retry the whole
// transaction. On MySQL a duplicate-key error rolls back only its statement.
func (s *Store[P]) Mark(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	if err := checkKey(item.Key); err != nil {
		return retryqueue.Item[P]{}, err
	}
	payload, err := s.encode(item.Payload)
	if err != nil {
		return retryqueue.Item[P]{}, err
	}
	row, err := s.mark(ctx, item, payload)
	if err != nil {
		return retryqueue.Item[P]{}, err
	}
	return row.decode(s.codec)
}

// mark keeps the stored payload encoded so Reopen can replace an older schema
// using the row's state, generation and age alone.
func (s *Store[P]) mark(ctx context.Context, item retryqueue.Item[P], payload []byte) (storedRow[P], error) {
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	for range markAttempts {
		inserted, err := s.insert(ctx, item, payload)
		if err != nil {
			return storedRow[P]{}, coreerrs.WrapOperation(err, "mark retry item")
		}
		if inserted {
			return freshRow(item, payload), nil
		}
		got, err := scanRow[P](s.db.QueryRowContext(ctx, s.stmts.getMark, s.dialect.textArg(item.Key)))
		switch {
		case err == nil:
			return got, nil
		case !errors.Is(err, sql.ErrNoRows):
			return storedRow[P]{}, coreerrs.WrapOperation(err, "read marked retry item")
		}
		// Deleted between the insert and the read: insert again.
	}
	return storedRow[P]{}, coreerrs.WrapOperation(errors.New("item deleted concurrently"), "mark retry item")
}

// insert inserts item as a fresh pending row and reports whether it did; a
// taken key is not an error.
func (s *Store[P]) insert(ctx context.Context, item retryqueue.Item[P], payload []byte) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.stmts.insert, s.dialect.textArg(item.Key), item.Generation,
		unixNano(item.Since), unixNano(item.NextAttemptAt), payload)
	if err != nil {
		if s.dialect.name == DialectMySQL && sqldialect.IsDuplicateKey(err) {
			return false, nil
		}
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Reopen implements [retryqueue.Storage] with an UPDATE that replaces every
// column of the matched row, so nothing of the previous generation survives.
// When no row matched (the item is newer, already pending at item's
// generation, or missing) it falls back to [Store.Mark]. If Mark returns an
// older generation or a closed item at the same generation, it retries the
// update, for at most two rounds. Persistent contention returns the last
// stored item; the caller compares generations per the Storage contract.
func (s *Store[P]) Reopen(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	if err := checkKey(item.Key); err != nil {
		return retryqueue.Item[P]{}, err
	}
	payload, err := s.encode(item.Payload)
	if err != nil {
		return retryqueue.Item[P]{}, err
	}
	next := item
	var out storedRow[P]
	for range reopenAttempts {
		// A matched row always changes (its generation or its state), so the
		// affected-rows count is exact whatever the MySQL found-rows flag.
		n, err := s.exec(ctx, s.stmts.reopen, item.Generation, unixNano(next.Since), unixNano(item.NextAttemptAt), payload,
			s.dialect.textArg(item.Key), item.Generation, item.Generation, unixNano(next.Since))
		if err != nil {
			return retryqueue.Item[P]{}, coreerrs.WrapOperation(err, "reopen retry item")
		}
		if n > 0 {
			return freshRow(next, payload).decode(s.codec)
		}
		out, err = s.mark(ctx, item, payload)
		if err != nil {
			return retryqueue.Item[P]{}, err
		}
		if out.Generation > item.Generation || (out.Generation == item.Generation && out.State == retryqueue.StatePending) {
			break
		}
		if out.Generation == item.Generation && unixNano(item.Since) <= unixNano(out.Since) {
			// Reuse the existing Expire signature: an earlier snapshot must never
			// have the age of the reopened item, even on a frozen caller clock.
			nanos := unixNano(out.Since)
			if nanos == math.MaxInt64 {
				return retryqueue.Item[P]{}, errors.New("reopen retry item: age exceeds SQL timestamp range")
			}
			next.Since = fromUnixNano(nanos + 1)
		}
	}
	return out.decode(s.codec)
}

// ListDue implements [retryqueue.Storage].
func (s *Store[P]) ListDue(ctx context.Context, now time.Time, limit int) ([]retryqueue.Item[P], error) {
	return s.list(ctx, "list due retry items", s.stmts.listDue, unixNano(now), listLimit(limit))
}

// WakeDue implements [retryqueue.Storage] with one UPDATE over the pending
// items scheduled after now.
func (s *Store[P]) WakeDue(ctx context.Context, now time.Time) error {
	at := unixNano(now)
	if _, err := s.exec(ctx, s.stmts.wakeDue, at, at); err != nil {
		return coreerrs.WrapOperation(err, "wake due retry items")
	}
	return nil
}

// Reschedule implements [retryqueue.Storage].
func (s *Store[P]) Reschedule(ctx context.Context, key string, generation int64, next time.Time) error {
	return s.reschedule(ctx, key, generation, nil, next)
}

// RescheduleAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) RescheduleAttempt(ctx context.Context, attempt retryqueue.Attempt, next time.Time) error {
	return s.reschedule(ctx, attempt.Key, attempt.Generation, &attempt.Since, next)
}

func (s *Store[P]) reschedule(ctx context.Context, key string, generation int64, since *time.Time, next time.Time) error {
	if _, err := s.execFenced(ctx, s.stmts.reschedule, since, unixNano(next), s.dialect.textArg(key), generation); err != nil {
		return coreerrs.WrapOperation(err, "reschedule retry item")
	}
	return nil
}

// Expire implements [retryqueue.Storage].
func (s *Store[P]) Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error) {
	n, err := s.exec(ctx, s.stmts.expire, unixNano(at), s.dialect.textArg(key), generation, unixNano(since))
	if err != nil {
		return false, coreerrs.WrapOperation(err, "expire retry item")
	}
	return n > 0, nil
}

// Resolve implements [retryqueue.Storage].
func (s *Store[P]) Resolve(ctx context.Context, key string, generation int64, at time.Time) error {
	return s.resolve(ctx, key, generation, nil, at)
}

// ResolveAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) ResolveAttempt(ctx context.Context, attempt retryqueue.Attempt, at time.Time) error {
	return s.resolve(ctx, attempt.Key, attempt.Generation, &attempt.Since, at)
}

func (s *Store[P]) resolve(ctx context.Context, key string, generation int64, since *time.Time, at time.Time) error {
	if _, err := s.execFenced(ctx, s.stmts.resolve, since, unixNano(at), s.dialect.textArg(key), generation); err != nil {
		return coreerrs.WrapOperation(err, "resolve retry item")
	}
	return nil
}

// Flag implements [retryqueue.Storage].
func (s *Store[P]) Flag(ctx context.Context, key string, generation int64, reason string) (bool, error) {
	return s.flag(ctx, key, generation, nil, reason)
}

// FlagAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) FlagAttempt(ctx context.Context, attempt retryqueue.Attempt, reason string) (bool, error) {
	return s.flag(ctx, attempt.Key, attempt.Generation, &attempt.Since, reason)
}

func (s *Store[P]) flag(ctx context.Context, key string, generation int64, since *time.Time, reason string) (bool, error) {
	n, err := s.execFenced(ctx, s.stmts.flag, since, []byte(reason), s.dialect.textArg(key), generation, []byte(reason))
	if err != nil {
		return false, coreerrs.WrapOperation(err, "flag retry item")
	}
	return n > 0, nil
}

// ClearFlag implements [retryqueue.Storage].
func (s *Store[P]) ClearFlag(ctx context.Context, key string, generation int64) error {
	return s.clearFlag(ctx, key, generation, nil)
}

// ClearAttemptFlag implements [retryqueue.AttemptStorage].
func (s *Store[P]) ClearAttemptFlag(ctx context.Context, attempt retryqueue.Attempt) error {
	return s.clearFlag(ctx, attempt.Key, attempt.Generation, &attempt.Since)
}

func (s *Store[P]) clearFlag(ctx context.Context, key string, generation int64, since *time.Time) error {
	if _, err := s.execFenced(ctx, s.stmts.unflag, since, s.dialect.textArg(key), generation); err != nil {
		return coreerrs.WrapOperation(err, "clear retry item flag")
	}
	return nil
}

// DeleteObsolete implements [retryqueue.Storage]: a conditional delete, then,
// when nothing was deleted, a look-up of a newer item.
func (s *Store[P]) DeleteObsolete(ctx context.Context, key string, generation int64) (bool, error) {
	n, err := s.exec(ctx, s.stmts.deleteObsolete, s.dialect.textArg(key), generation)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "delete obsolete retry item")
	}
	if n > 0 {
		return false, nil
	}
	newer, err := s.found(ctx, s.stmts.newer, s.dialect.textArg(key), generation)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "look up newer retry item")
	}
	return newer, nil
}

// Delete implements [retryqueue.Storage].
func (s *Store[P]) Delete(ctx context.Context, key string) error {
	if _, err := s.exec(ctx, s.stmts.remove, s.dialect.textArg(key)); err != nil {
		return coreerrs.WrapOperation(err, "delete retry item")
	}
	return nil
}

// ListByState implements [retryqueue.Storage].
func (s *Store[P]) ListByState(ctx context.Context, state retryqueue.State, limit int) ([]retryqueue.Item[P], error) {
	return s.list(ctx, "list retry items by state", s.stmts.listByState, s.dialect.textArg(string(state)), listLimit(limit))
}

// Requeue implements [retryqueue.Storage]. Keys are listed at most
// [keyChunk] per statement; a longer list runs atomically in one transaction,
// using the caller's transaction when present and starting one otherwise.
func (s *Store[P]) Requeue(ctx context.Context, now time.Time, keys ...string) (int, error) {
	if bound, _ := s.db.Tx(ctx); bound == nil && len(keys) > keyChunk {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, coreerrs.WrapOperation(err, "begin requeue retry items")
		}
		defer func() { _ = tx.Rollback() }()
		n, err := s.Requeue(WithTx(ctx, tx), now, keys...)
		if err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, coreerrs.WrapOperation(err, "commit requeue retry items")
		}
		return n, nil
	}
	at := unixNano(now)
	if len(keys) == 0 {
		n, err := s.exec(ctx, s.dialect.bind(s.stmts.requeue), at, at)
		if err != nil {
			return 0, coreerrs.WrapOperation(err, "requeue retry items")
		}
		return int(n), nil
	}
	total := 0
	for start := 0; start < len(keys); start += keyChunk {
		chunk := keys[start:min(start+keyChunk, len(keys))]
		args := make([]any, 0, len(chunk)+2) //nolint:mnd // since and next attempt
		args = append(args, at, at)
		for _, k := range chunk {
			args = append(args, s.dialect.textArg(k))
		}
		query := s.dialect.bind(s.stmts.requeue + " AND id IN (?" + strings.Repeat(", ?", len(chunk)-1) + ")")
		n, err := s.exec(ctx, query, args...)
		if err != nil {
			return total, coreerrs.WrapOperation(err, "requeue retry items")
		}
		total += int(n)
	}
	return total, nil
}

// Move implements [retryqueue.Mover] with one UPDATE of the primary key, so
// every other column is kept as stored and no reader sees the item under both
// keys or under neither. It looks both keys up before writing — from first,
// so a missing source is reported as such whatever to holds — and so a
// refusal leaves a caller transaction ([WithTx]) usable; only a concurrent
// insert of to between the check and the UPDATE fails the UPDATE, and the
// duplicate-key error, reported as [retryqueue.ErrItemExists], aborts a
// PostgreSQL transaction.
func (s *Store[P]) Move(ctx context.Context, from, to string) error {
	found, err := s.found(ctx, s.stmts.exists, s.dialect.textArg(from))
	switch {
	case err != nil:
		return coreerrs.WrapOperation(err, "look up retry item to move")
	case !found:
		return fmt.Errorf("move %q: %w", from, retryqueue.ErrItemNotFound)
	case from == to:
		return nil
	}
	if err = checkKey(to); err != nil {
		return err
	}
	taken, err := s.found(ctx, s.stmts.exists, s.dialect.textArg(to))
	switch {
	case err != nil:
		return coreerrs.WrapOperation(err, "look up retry item move target")
	case taken:
		return fmt.Errorf("move %q to %q: %w", from, to, retryqueue.ErrItemExists)
	}
	n, err := s.exec(ctx, s.stmts.move, s.dialect.textArg(to), s.dialect.textArg(from))
	switch {
	case err != nil && sqldialect.IsDuplicateKey(err):
		return fmt.Errorf("move %q to %q: %w: %w", from, to, retryqueue.ErrItemExists, err)
	case err != nil:
		return coreerrs.WrapOperation(err, "move retry item")
	case n == 0:
		// Moved or deleted concurrently since the look-up.
		return fmt.Errorf("move %q: %w", from, retryqueue.ErrItemNotFound)
	}
	return nil
}

// PurgeExpired deletes the resolved items whose ResolvedAt is older than the
// resolved retention ([WithResolvedRetention]), compared against the database
// clock at whole seconds, and returns how many it deleted. It is the
// counterpart of the MongoDB store's TTL index: call it periodically, for
// example once per sweep or from a scheduler, or let the store run it with
// [WithPurgeInterval]. Without a retention it deletes nothing and performs
// no I/O. An item resolved at the zero time is never purged. It deletes in
// batches bounded by WithPurgeBatchSize; concurrent purges take disjoint
// chunks on PostgreSQL. Each statement is bounded by
// purgeloop.DefaultStepTimeout (30 seconds).
func (s *Store[P]) PurgeExpired(ctx context.Context) (int64, error) {
	if s.retention <= 0 {
		return 0, nil
	}
	total, err := purgeloop.Drain(ctx, s.purgeBatchSize, purgeloop.DefaultStepTimeout,
		func(ctx context.Context) (int64, int64, error) {
			res, err := s.db.ExecContext(ctx, s.stmts.purge, s.retention.Nanoseconds())
			if err != nil {
				return 0, 0, err
			}
			n, err := res.RowsAffected()
			return n, n, err
		})
	return total, coreerrs.WrapOperation(err, "purge resolved retry items")
}

// exec runs a statement and returns its affected-rows count.
func (s *Store[P]) execFenced(ctx context.Context, query fencedQuery, since *time.Time, args ...any) (int64, error) {
	if since == nil {
		return s.exec(ctx, query.generation, args...)
	}
	return s.exec(ctx, query.attempt, append(args, unixNano(*since))...)
}

func (s *Store[P]) exec(ctx context.Context, query string, args ...any) (int64, error) {
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// found reports whether query returns a row.
func (s *Store[P]) found(ctx context.Context, query string, args ...any) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// list runs a sorted, limited query and scans its rows; op names the
// operation in errors.
func (s *Store[P]) list(ctx context.Context, op, query string, args ...any) ([]retryqueue.Item[P], error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, op)
	}
	defer func() { _ = rows.Close() }()

	out := []retryqueue.Item[P]{}
	for rows.Next() {
		row, err := scanRow[P](rows)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "scan: "+op)
		}
		it, err := row.decode(s.codec)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "decode: "+op)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, coreerrs.WrapOperation(err, op)
	}
	return out, nil
}

// scanner is the part of *sql.Row and *sql.Rows scanRow uses.
type scanner interface {
	Scan(dest ...any) error
}

// storedRow holds the payload bytes until the item is returned to a caller.
type storedRow[P any] struct {
	retryqueue.Item[P]
	encoded []byte
}

// scanRow reads one row selected with columns.
func scanRow[P any](sc scanner) (storedRow[P], error) {
	var (
		it                                         storedRow[P]
		state                                      string
		since, attempts, next, expiredAt, resolved int64
		reason                                     []byte
	)
	if err := sc.Scan(&it.Key, &it.Generation, &state, &since, &attempts, &next, &expiredAt, &resolved,
		&it.Flagged, &reason, &it.encoded); err != nil {
		return storedRow[P]{}, err
	}
	it.State = retryqueue.State(state)
	it.Since, it.NextAttemptAt = fromUnixNano(since), fromUnixNano(next)
	it.ExpiredAt, it.ResolvedAt = fromUnixNano(expiredAt), fromUnixNano(resolved)
	it.Attempts = int(attempts)
	it.FlagReason = string(reason)
	return it, nil
}

// freshRow constructs exactly what a newly inserted or reopened row contains.
func freshRow[P any](item retryqueue.Item[P], payload []byte) storedRow[P] {
	fresh := retryqueue.Item[P]{
		Key:           item.Key,
		Generation:    item.Generation,
		State:         retryqueue.StatePending,
		Since:         fromUnixNano(unixNano(item.Since)),
		NextAttemptAt: fromUnixNano(unixNano(item.NextAttemptAt)),
	}
	return storedRow[P]{Item: fresh, encoded: payload}
}

// encode renders a payload for the payload column; an empty encoding is
// stored as empty bytes (the column is NOT NULL).
func (s *Store[P]) encode(p P) ([]byte, error) {
	b, err := s.codec.Serialize(p)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "encode retry item payload")
	}
	if b == nil {
		b = []byte{}
	}
	return b, nil
}

// decode returns the row with the codec's payload; empty bytes represent zero.
func (row storedRow[P]) decode(codec serializer.Serializer) (retryqueue.Item[P], error) {
	if len(row.encoded) > 0 {
		if err := codec.Deserialize(row.encoded, &row.Payload); err != nil {
			return retryqueue.Item[P]{}, coreerrs.WrapOperation(err, "decode retry item payload")
		}
	}
	return row.Item, nil
}

// checkKey rejects a key longer than [MaxKeyLength] characters.
func checkKey(key string) error {
	if n := utf8.RuneCountInString(key); n > MaxKeyLength {
		return fmt.Errorf("%w: key of %d characters, limit %d", ErrValueTooLong, n, MaxKeyLength)
	}
	return nil
}

// listLimit maps a limit that is not positive to [retryqueue.DefaultListLimit].
func listLimit(limit int) int {
	if limit <= 0 {
		return retryqueue.DefaultListLimit
	}
	return limit
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
