// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/internal/watermark"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Dialect selects a database/sql backend.
type Dialect string

const (
	// DialectPostgres supports PostgreSQL.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL supports MySQL and MariaDB with InnoDB.
	DialectMySQL Dialect = "mysql"
)

var (
	// ErrUnsupportedDialect indicates an unknown SQL dialect.
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName indicates an invalid SQL identifier.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
)

// Storage persists request ID reservations and their replay outcomes.
type Storage struct {
	db             *sql.DB
	style          sqldialect.Style
	table          string
	name           string
	insert         string
	observe        string
	takeover       string
	complete       string
	release        string
	cleanup        string
	purgeBatchSize int
	purge          *purgeloop.Loop
	clock          watermark.Clock
}

var _ requestid.Store = (*Storage)(nil)

// New validates the storage configuration. Call [Storage.EnsureSchema] or apply its DDL before use.
func New(db *sql.DB, dialect Dialect, opts ...Option) (*Storage, error) {
	if db == nil {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "requestidstore: database is required")
	}
	var style sqldialect.Style
	switch dialect {
	case DialectPostgres:
		style = sqldialect.Postgres
	case DialectMySQL:
		style = sqldialect.MySQL
	default:
		return nil, coreerrs.Wrapf(ErrUnsupportedDialect, "%q", dialect)
	}
	o := newOptions(opts...)
	table, err := style.Table(o.tableName)
	if err != nil {
		return nil, err
	}
	s := &Storage{db: db, style: style, table: table, name: sqldialect.Unqualified(o.tableName), purgeBatchSize: o.purgeBatchSize}
	s.buildStatements()
	s.purge = purgeloop.New(o.purgeInterval, s.PurgeExpired, o.logger, "expired request IDs")
	return s, nil
}

const entryColumns = "state, token, fingerprint, response, failure, response_omitted, expires_at"

func (s *Storage) buildStatements() {
	t := s.table
	bind := func(q string) string { return s.style.Bind(q, 1) }
	owner := " WHERE request_key = ? AND state = ? AND token = ? AND expires_at > ?"
	insert := "INSERT INTO " + t + " (" + entryColumns + ", request_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
	// A revision increment also counts an otherwise identical takeover as a changed row on MySQL.
	update := "UPDATE " + t + " SET state = ?, token = ?, fingerprint = ?, response = ?, failure = ?," +
		" response_omitted = ?, expires_at = ?, revision = revision + 1"
	cleanup := "DELETE FROM " + t + " WHERE expires_at <= ? ORDER BY expires_at, request_key LIMIT ?"
	if s.style == sqldialect.Postgres {
		insert += " ON CONFLICT (request_key) DO NOTHING"
		cleanup = "DELETE FROM " + t + " WHERE request_key IN (SELECT request_key FROM " + t +
			" WHERE expires_at <= ? ORDER BY expires_at, request_key LIMIT ? FOR UPDATE SKIP LOCKED)"
	}
	s.insert = bind(insert)
	s.observe = bind("SELECT " + entryColumns + " FROM " + t + " WHERE request_key = ? AND expires_at > ?")
	s.takeover = bind(update + " WHERE request_key = ? AND expires_at <= ?")
	s.complete = bind(update + owner)
	s.release = bind("DELETE FROM " + t + owner)
	s.cleanup = bind(cleanup)
}

func (s *Storage) observeTime(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.clock.Observe(now)
	s.purge.Start() //nolint:contextcheck // Background collection has an independent lifetime.
	return nil
}

func entryArgs(e requestid.Entry, key string) []any {
	return []any{e.State, []byte(e.Token), e.Fingerprint, e.Response, e.Failure, e.ResponseOmitted, e.ExpiresAt.UnixMilli(), key}
}

func (s *Storage) exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Reserve acquires an absent or expired key, or returns its live entry.
func (s *Storage) Reserve(ctx context.Context, key string, pending requestid.Entry, now time.Time) (requestid.Entry, bool, error) {
	if err := s.observeTime(ctx, now); err != nil {
		return requestid.Entry{}, false, err
	}
	args := entryArgs(pending, key)
	for {
		n, err := s.exec(ctx, s.insert, args...)
		if err != nil && !sqldialect.IsDuplicateKey(err) {
			return requestid.Entry{}, false, coreerrs.WrapOperation(err, "insert request ID reservation")
		}
		if n > 0 {
			return pending, true, nil
		}
		var entry requestid.Entry
		var expires int64
		err = s.db.QueryRowContext(ctx, s.observe, key, now.UnixMilli()).Scan(
			&entry.State, &entry.Token, &entry.Fingerprint, &entry.Response, &entry.Failure, &entry.ResponseOmitted, &expires)
		switch {
		case err == nil:
			entry.ExpiresAt = time.UnixMilli(expires).UTC()
			return entry, false, nil
		case !errors.Is(err, sql.ErrNoRows):
			return requestid.Entry{}, false, coreerrs.WrapOperation(err, "read live request ID")
		}
		n, err = s.exec(ctx, s.takeover, append(args, now.UnixMilli())...)
		if err != nil {
			return requestid.Entry{}, false, coreerrs.WrapOperation(err, "replace expired request ID")
		}
		if n > 0 {
			return pending, true, nil
		}
		// A release, purge, or another reservation raced the read. Retry from insert.
	}
}

// Complete atomically replaces a live pending reservation owned by token with done.
func (s *Storage) Complete(ctx context.Context, key, token string, done requestid.Entry, now time.Time) error {
	if err := s.observeTime(ctx, now); err != nil {
		return err
	}
	args := append(entryArgs(done, key), requestid.StatePending, []byte(token), now.UnixMilli())
	n, err := s.exec(ctx, s.complete, args...)
	return owned(n, coreerrs.WrapOperation(err, "complete request ID"))
}

// Release deletes the matching owner's live pending reservation.
func (s *Storage) Release(ctx context.Context, key, token string, now time.Time) error {
	if err := s.observeTime(ctx, now); err != nil {
		return err
	}
	n, err := s.exec(ctx, s.release, key, requestid.StatePending, []byte(token), now.UnixMilli())
	return owned(n, coreerrs.WrapOperation(err, "release request ID"))
}

func owned(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return requestid.ErrReservationLost
	}
	return nil
}

// PurgeExpired removes rows expired as of the latest observed caller time, in bounded batches.
func (s *Storage) PurgeExpired(ctx context.Context) (int64, error) {
	now := s.clock.Now()
	if now.IsZero() {
		return 0, nil
	}
	cutoff := now.UnixMilli()
	total, err := purgeloop.Drain(ctx, s.purgeBatchSize, purgeloop.DefaultStepTimeout, func(stepCtx context.Context) (int64, int64, error) {
		n, err := s.exec(stepCtx, s.cleanup, cutoff, s.purgeBatchSize)
		return n, n, err
	})
	return total, coreerrs.WrapOperation(err, "purge expired request IDs")
}

// Close stops the background purge loop. The caller owns the database connection pool.
func (s *Storage) Close(ctx context.Context) error { return s.purge.Close(ctx) }
