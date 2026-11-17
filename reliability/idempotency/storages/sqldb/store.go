// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Storage persists idempotency bytes with database-clock expiry and byte CAS.
// It is safe for concurrent use. The caller owns the database and its lifetime.
type Storage struct {
	db             *sql.DB
	dialect        dialect
	table          string
	name           string
	ttl            time.Duration
	stmts          statements
	purgeBatchSize int
	purge          *purgeloop.Loop
}

var _ storages.Storage = (*Storage)(nil)

type statements struct {
	insert, takeover, observe, complete, upsert, release, remove, cleanup, expiredKeys string
}

// New validates the configuration without I/O. Call EnsureSchema separately.
func New(db *sql.DB, d Dialect, opts ...Option) (*Storage, error) {
	if db == nil {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "sqldb: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)
	if d == DialectMySQL && o.purgeBatchSize > sqldialect.MaxKeyBatch {
		return nil, coreerrs.Wrapf(coreerrs.ErrInvalidArgument, "sqldb: MySQL purge batch size must be at most %d", sqldialect.MaxKeyBatch)
	}
	table, err := dl.Table(o.tableName)
	if err != nil {
		return nil, err
	}
	s := &Storage{db: db, dialect: dl, table: table, name: sqldialect.Unqualified(o.tableName), ttl: o.ttl,
		purgeBatchSize: o.purgeBatchSize}
	s.stmts = s.buildStatements()
	s.purge = purgeloop.New(o.purgeInterval, s.PurgeExpired, o.logger, "expired idempotency keys")
	return s, nil
}

func (s *Storage) buildStatements() statements {
	d, t := s.dialect, s.table
	k := d.Ident("key")
	live := " AND expires_at > " + d.now
	insert := "INSERT INTO " + t + " (" + k + ", val, expires_at) VALUES (?, ?, " + d.expiresIn() + ")"
	st := statements{
		insert: insert + " ON CONFLICT (" + k + ") DO NOTHING",
		takeover: "UPDATE " + t + " SET val = ?, expires_at = " + d.expiresIn() + ", revision = revision + 1" +
			" WHERE " + k + " = ? AND expires_at <= " + d.now,
		observe: "SELECT val FROM " + t + " WHERE " + k + " = ?" + live + " FOR UPDATE",
		complete: "UPDATE " + t + " SET val = ?, expires_at = " + d.expiresIn() + ", revision = revision + 1" +
			" WHERE " + k + " = ? AND val = ?" + live,
		upsert: insert + " ON CONFLICT (" + k + ") DO UPDATE SET val = EXCLUDED.val, expires_at = EXCLUDED.expires_at, revision = " +
			d.Ident(s.name) + ".revision + 1",
		release: "DELETE FROM " + t + " WHERE " + k + " = ? AND val = ?" + live,
		remove:  "DELETE FROM " + t + " WHERE " + k + " = ?",
		cleanup: "DELETE FROM " + t + " WHERE " + k + " IN (SELECT " + k + " FROM " + t +
			" WHERE expires_at <= " + d.now + " ORDER BY expires_at, " + k + " LIMIT ? FOR UPDATE SKIP LOCKED)",
	}
	if d.name == DialectMySQL {
		st.insert = "INSERT IGNORE INTO " + t + " (" + k + ", val, expires_at) VALUES (?, ?, " + d.expiresIn() + ")"
		st.upsert = insert + " ON DUPLICATE KEY UPDATE val = VALUES(val), expires_at = VALUES(expires_at), revision = revision + 1"
		st.expiredKeys = "SELECT " + k + " FROM " + t + " WHERE expires_at <= " + d.now + " ORDER BY expires_at, " + k + " LIMIT ?"
		st.cleanup = "DELETE FROM " + t + " WHERE " + k + " IN (%s) AND expires_at <= " + d.now
	}
	st.insert, st.takeover, st.observe = d.bind(st.insert), d.bind(st.takeover), d.bind(st.observe)
	st.complete, st.upsert = d.bind(st.complete), d.bind(st.upsert)
	st.release, st.remove, st.cleanup = d.bind(st.release), d.bind(st.remove), d.bind(st.cleanup)
	return st
}

type txKey struct{}

// WithTx makes all key operations join tx, so keys commit or abort with the
// caller's business writes. The caller owns commit, rollback and transaction
// retries. The transaction must belong to the database passed to New.
// ErrRestartTransaction signals that the whole business transaction must restart.
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Storage) executor(ctx context.Context) executor {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return s.db
}

func validateKey(key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if utf8.RuneCountInString(key) > MaxKeyLength {
		return ErrValueTooLong
	}
	return nil
}

// AttemptLock acquires a missing or expired key with the configured TTL.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return s.AttemptLockWithTTL(ctx, key, val, 0)
}

// AttemptLockWithTTL acquires a key with a positive per-call TTL override.
// A losing caller receives the stored bytes and no token. A winning caller
// receives a copy of val as its CAS token, even when competing values are equal.
func (s *Storage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTTL time.Duration) (bool, []byte, []byte, error) {
	if err := validateKey(key); err != nil {
		return false, nil, nil, err
	}
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	if lockTTL <= 0 {
		lockTTL = s.ttl
	}
	val = nonNilBytes(val)
	ex := s.executor(ctx)
	arg, ttl := s.dialect.keyArg(key), s.dialect.duration(lockTTL)
	for {
		if err := ctx.Err(); err != nil {
			return false, nil, nil, err
		}
		n, err := execCount(ctx, ex, s.stmts.insert, arg, val, ttl)
		if err != nil {
			return false, nil, nil, operationError(ctx, err, "insert idempotency lock in SQL")
		}
		if n == 0 {
			n, err = execCount(ctx, ex, s.stmts.takeover, val, ttl, arg)
			if err != nil {
				return false, nil, nil, operationError(ctx, err, "take expired idempotency lock in SQL")
			}
		}
		if n > 0 {
			return true, nil, slices.Clone(val), nil
		}
		var existing []byte
		err = ex.QueryRowContext(ctx, s.stmts.observe, arg).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			// Release, cleanup or expiry raced the write and the read. Try
			// acquisition again. A locking read avoids a stale MySQL snapshot
			// when this operation joins a REPEATABLE READ business transaction.
			continue
		}
		return false, existing, nil, operationError(ctx, err, "read idempotency lock in SQL")
	}
}

// Complete writes a result with the configured TTL when the live stored bytes
// match lockToken. A nil token unconditionally inserts or overwrites the key.
func (s *Storage) Complete(ctx context.Context, key string, val, lockToken []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	if lockToken == nil {
		_, err := s.executor(ctx).ExecContext(ctx, s.stmts.upsert, s.dialect.keyArg(key), nonNilBytes(val), s.dialect.duration(s.ttl))
		return operationError(ctx, err, "complete idempotency key in SQL")
	}
	return s.compareAndSet(ctx, key, lockToken, val)
}

// Steal replaces live expectedVal bytes and returns newVal as the new token.
// A missing, expired or changed key returns storages.ErrLockStolen.
func (s *Storage) Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	if err := s.compareAndSet(ctx, key, expectedVal, newVal); err != nil {
		return nil, err
	}
	return slices.Clone(nonNilBytes(newVal)), nil
}

func (s *Storage) compareAndSet(ctx context.Context, key string, expected, val []byte) error {
	// Incrementing revision guarantees a changed row on MySQL even when
	// value and expiry are unchanged at the database's timestamp precision.
	n, err := execCount(ctx, s.executor(ctx), s.stmts.complete, nonNilBytes(val), s.dialect.duration(s.ttl),
		s.dialect.keyArg(key), nonNilBytes(expected))
	return casResult(n, operationError(ctx, err, "compare and set idempotency key in SQL"))
}

// Release deletes only a live key whose bytes match lockToken.
func (s *Storage) Release(ctx context.Context, key string, lockToken []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	n, err := execCount(ctx, s.executor(ctx), s.stmts.release, s.dialect.keyArg(key), lockToken)
	return casResult(n, operationError(ctx, err, "release idempotency key in SQL"))
}

// Delete unconditionally removes key for administrative invalidation.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.purge.Start() //nolint:contextcheck // Background purging carries no request context.
	_, err := s.executor(ctx).ExecContext(ctx, s.stmts.remove, s.dialect.keyArg(key))
	return operationError(ctx, err, "delete idempotency key from SQL")
}

// PurgeExpired deletes all currently expired keys in bounded batches using
// the database clock. It returns completed deletions even if a later batch
// fails. Each statement is bounded by purgeloop.DefaultStepTimeout (30 seconds).
// Explicit calls join WithTx; background passes use the database.
func (s *Storage) PurgeExpired(ctx context.Context) (int64, error) {
	total, err := purgeloop.Drain(ctx, s.purgeBatchSize, purgeloop.DefaultStepTimeout, s.deleteExpiredBatch)
	return total, operationError(ctx, err, "purge expired idempotency keys from SQL")
}

func execCount(ctx context.Context, ex executor, query string, args ...any) (int64, error) {
	res, err := ex.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func casResult(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return storages.ErrLockStolen
	}
	return nil
}

func nonNilBytes(v []byte) []byte {
	if v == nil {
		return []byte{}
	}
	return v
}
