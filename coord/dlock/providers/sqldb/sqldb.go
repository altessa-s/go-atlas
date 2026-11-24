// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/internal/leasing"
	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ensureAttempts bounds the attempts of the MySQL statement that creates a
// key's row: InnoDB may resolve concurrent first uses of one key as a
// deadlock, which a retry settles. It also bounds the MySQL acquisitions
// restarted because the expired-row cleanup deleted the key's row between
// creating and taking it.
const ensureAttempts = 3

// Locker is a [providers.Provider] keeping each lock as a leased row in a SQL
// table. A lock is taken by a conditional write that succeeds only while the
// key has no unexpired lease, renewed in the background, and released when
// its context ends or Release is called. Lease expiry is judged by the
// database clock, so replicas with skewed clocks agree. The fencing token
// grows by one with every acquisition of a key.
//
// Rows are kept after release (one per key ever locked) so fencing tokens
// stay monotonic, unless [WithExpiredCleanup] lets the locker delete them a
// grace period after their lease ended; deleting rows otherwise, or restoring
// the table from a backup, can reissue tokens. [WithoutRenewal] turns every
// lease into a hard lease of one TTL that is never renewed. The caller owns
// the *sql.DB. WithTxResolver can enlist lock operations in a caller transaction.
type Locker struct {
	s            *store
	engine       *leasing.Engine
	engineClosed atomic.Bool
	// opTimeout bounds one cleanup statement.
	opTimeout time.Duration

	sweep *purgeloop.Loop
}

// store implements [leasing.Store] on the locks table.
type store struct {
	db        sqldialect.Pool
	dialect   dialect
	table     string // quoted
	tableName string // unqualified, for the DDL lock
	stmts     statements
	ttl       time.Duration
	// cleanup enables [WithExpiredCleanup]; graceUs is its grace in
	// microseconds.
	cleanup        bool
	graceUs        int64
	purgeBatchSize int
}

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	acquire, ensure, take, fencing, lockRow string
	read, readOwned                         string
	renew, release, releaseOwner            string
	// exists checks a MySQL key's row inside the acquisition transaction.
	exists string
	// deleteExpired deletes one batch of rows whose lease ended more than the
	// cleanup grace ago; on MySQL expiredKeys selects the batch first and
	// deleteExpired takes the key list through %s.
	expiredKeys, deleteExpired string
}

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
	_ leasing.Store      = (*store)(nil)
)

// New creates a SQL lock provider over db for the given dialect. It performs
// no I/O; call [Locker.EnsureSchema] once at startup to create the table, or
// apply the equivalent DDL through a migration tool.
//
// Example:
//
//	db, _ := sql.Open("pgx", dsn)
//	locker, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTTL(15*time.Second))
//	if err != nil {
//		return err
//	}
//	if err := locker.EnsureSchema(ctx); err != nil {
//		return err
//	}
func New(db *sql.DB, d Dialect, opts ...Option) (*Locker, error) {
	if db == nil {
		return nil, errors.New("sqldb dlock: database is required")
	}
	dl, err := dialectFor(d)
	if err != nil {
		return nil, err
	}
	o := newOptions(opts...)
	// A hard lease has no renewal, so its ratio plays no part.
	var ttl, interval time.Duration
	if o.withoutRenewal {
		ttl, err = leasing.HardTTL("sqldb dlock", o.ttl)
	} else {
		ttl, interval, err = leasing.Timing("sqldb dlock", o.ttl, o.renewRatio)
	}
	if err != nil {
		return nil, err
	}
	if d == DialectMySQL && o.purgeBatchSize > sqldialect.MaxKeyBatch {
		return nil, coreerrs.Wrapf(coreerrs.ErrInvalidArgument, "sqldb dlock: MySQL purge batch size must be at most %d", sqldialect.MaxKeyBatch)
	}
	if o.expiredCleanup && o.expiredCleanupGrace < 0 {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "sqldb dlock: expired cleanup grace must not be negative")
	}
	s := &store{db: dl.WrapPool(db).WithResolver(o.txResolver), dialect: dl, tableName: sqldialect.Unqualified(o.tableName), ttl: ttl,
		cleanup: o.expiredCleanup, graceUs: o.expiredCleanupGrace.Microseconds(), purgeBatchSize: o.purgeBatchSize}
	if s.table, err = dl.Table(o.tableName); err != nil {
		return nil, err
	}
	s.stmts = s.buildStatements()
	l := &Locker{
		s: s,
		engine: leasing.New(s, leasing.Settings{
			Logger: o.logger, TTL: ttl, Interval: interval, OperationsTimeout: o.operationsTimeout,
			NoRenewal: o.withoutRenewal,
		}),
		opTimeout: o.operationsTimeout,
	}
	every := o.purgeInterval
	if !o.expiredCleanup {
		every = 0
	}
	l.sweep = purgeloop.New(every, l.PurgeExpired, o.logger, "expired locks")
	return l, nil
}

func (s *store) buildStatements() statements {
	t, now, b := s.table, s.dialect.now, s.dialect.Bind
	const cols = "lock_key, owner, fencing, acquired_at, renewed_at, ttl_us"
	unexpired := " AND expires_at > " + now
	st := statements{
		read:      b("SELECT "+cols+" FROM "+t+" WHERE lock_key = ?"+unexpired, 1),
		readOwned: b("SELECT "+cols+" FROM "+t+" WHERE lock_key = ? AND owner = ? AND fencing = ?"+unexpired, 1),
		// Only an unexpired lease is renewed: an expired one stays lost even
		// when nobody took it over.
		renew: b("UPDATE "+t+" SET renewed_at = "+now+", expires_at = "+now+" + ?"+
			" WHERE lock_key = ? AND owner = ? AND fencing = ?"+unexpired, 1),
		// A renewal that waits for the row lock must not compare the clock it
		// read before waiting: MySQL fixes UTC_TIMESTAMP when a statement
		// starts, and PostgreSQL rechecks an UPDATE's condition after a lock
		// wait only when the row changed meanwhile. Renewal therefore locks
		// the row first and reads the clock in the next statement.
		lockRow:      b("SELECT 1 FROM "+t+" WHERE lock_key = ? FOR UPDATE", 1),
		release:      b("UPDATE "+t+" SET owner = '', expires_at = "+now+" WHERE lock_key = ? AND owner = ? AND fencing = ?", 1),
		releaseOwner: b("UPDATE "+t+" SET owner = '', expires_at = "+now+" WHERE lock_key = ? AND owner = ?", 1),
	}
	// The cleanup cutoff reads the clock once per statement: a volatile clock
	// cannot bound an index scan, so PostgreSQL would read the whole table on
	// every sweep. Oldest rows go first, along the expires_at index.
	expired := "expires_at <= " + s.dialect.cutoffNow + " - ?"
	if s.dialect.name == DialectPostgres {
		// SKIP LOCKED lets concurrent sweeps take disjoint batches and leaves
		// rows an acquisition is writing alone; a row changed after the
		// statement started is rechecked against the condition when locked,
		// so a lease taken meanwhile is not deleted.
		st.deleteExpired = b("DELETE FROM "+t+" WHERE lock_key IN (SELECT lock_key FROM "+t+" WHERE "+expired+
			" ORDER BY expires_at LIMIT ? FOR UPDATE SKIP LOCKED)", 1)
		// One statement: insert the key's row on first use, or take the lease
		// over while it is expired. ON CONFLICT waits for a concurrent writer
		// of the row and re-evaluates the condition after it; no row comes
		// back when the key is held.
		st.acquire = b("INSERT INTO "+t+" AS cur (lock_key, owner, fencing, acquired_at, renewed_at, expires_at, ttl_us)"+
			" VALUES (?, ?, 1, "+now+", "+now+", "+now+" + ?, ?)"+
			" ON CONFLICT (lock_key) DO UPDATE SET owner = EXCLUDED.owner, fencing = cur.fencing + 1,"+
			" acquired_at = "+now+", renewed_at = "+now+", expires_at = "+now+" + EXCLUDED.ttl_us, ttl_us = EXCLUDED.ttl_us"+
			" WHERE cur.expires_at <= "+now+
			" RETURNING fencing", 1)
		return st
	}
	// MySQL: create the row on first use, then take it over in a conditional
	// UPDATE (the fencing increment always changes the row, so the affected
	// count is exact) and read the new token in the same transaction.
	st.ensure = "INSERT INTO " + t + " (lock_key) VALUES (?) ON DUPLICATE KEY UPDATE lock_key = lock_key"
	st.take = "UPDATE " + t + " SET owner = ?, fencing = fencing + 1, acquired_at = " + now + ", renewed_at = " + now +
		", expires_at = " + now + " + ?, ttl_us = ? WHERE lock_key = ? AND expires_at <= " + now
	st.fencing = "SELECT fencing FROM " + t + " WHERE lock_key = ?"
	st.exists = "SELECT 1 FROM " + t + " WHERE lock_key = ?"
	// MySQL cannot select from the table a DELETE modifies, and a DELETE that
	// scans for its condition locks every row it reads. So the sweep reads a
	// batch of keys without locking and deletes them through the primary key,
	// which locks only those rows; the condition is repeated because the read
	// took no lock.
	st.expiredKeys = "SELECT lock_key FROM " + t + " WHERE " + expired + " ORDER BY expires_at LIMIT ?"
	st.deleteExpired = "DELETE FROM " + t + " WHERE lock_key IN (%s) AND " + expired
	return st
}

// Lock makes a single attempt to take the lock for key and returns
// [providers.ErrLockNotHeld] when another holder has an unexpired lease on it, or
// [ErrValueTooLong] for a key longer than [MaxKeyLength] characters.
//
// ctx scopes the lock: while it lives the lease is renewed every
// TTL × renew ratio; when it ends the lease is released. A lease that cannot
// be renewed because it expired or another holder took the key over is
// reported lost (see [providers.Lock.GetLockInfo]); one that fails
// transiently is retried on the next renewal.
func (l *Locker) Lock(ctx context.Context, key string) (providers.Lock, error) {
	if n := utf8.RuneCountInString(key); n > MaxKeyLength {
		return nil, fmt.Errorf("%w: %d characters, limit %d", ErrValueTooLong, n, MaxKeyLength)
	}
	l.sweep.Start() //nolint:contextcheck // Background purging carries no request context.
	return l.engine.Lock(ctx, key)
}

// GetLockInfo returns the lease currently held on key, or
// [providers.ErrLockNotHeld] when the key has no unexpired lease.
func (l *Locker) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return l.engine.GetLockInfo(ctx, key)
}

// Close stops renewing every lock held through this provider, rejects
// further Lock calls, waits for acquisitions already in flight and releases
// every held lock. It also stops the background sweep of
// [WithExpiredCleanup], waiting for a sweep in progress after the releases.
// It returns the release failures and a failed wait for the sweep; calling Close again retries the cleanup of what is still
// held (as does each lock's Release). The *sql.DB is managed by the caller.
func (l *Locker) Close(ctx context.Context) error {
	// The sweep is told to stop first, but waited for last, so a sweep that
	// is slow to return never keeps held locks from being released.
	done := l.sweep.Stop()
	var engineErr error
	if !l.engineClosed.Load() {
		engineErr = l.engine.Close(ctx)
		if engineErr == nil {
			l.engineClosed.Store(true)
		}
	}
	return errors.Join(l.sweep.Wait(ctx, done), engineErr)
}

// Probe implements [providers.Prober]: it fails when the provider is closed
// or the database is unreachable.
func (l *Locker) Probe(ctx context.Context) error {
	return l.engine.Probe(ctx)
}

// Acquire implements [leasing.Store]: on PostgreSQL one upsert, on MySQL the
// row created on first use and then taken over in a transaction that reads
// the new token.
func (s *store) Acquire(ctx context.Context, key, owner string) (uint64, bool, error) {
	fencing, acquired, err := s.acquire(ctx, key, owner)
	if err != nil {
		return 0, false, coreerrs.WrapOperation(err, "acquire SQL lock")
	}
	return uint64(fencing), acquired, nil //nolint:gosec // fencing starts at 1 and only grows
}

func (s *store) acquire(ctx context.Context, key, owner string) (int64, bool, error) {
	ttl := s.ttl.Microseconds()
	k, o := s.dialect.textArg(key), s.dialect.textArg(owner)
	if s.dialect.name == DialectPostgres {
		var fencing int64
		err := s.db.QueryRowContext(ctx, s.stmts.acquire, k, o, ttl, ttl).Scan(&fencing)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return fencing, err == nil, err
	}

	for range ensureAttempts {
		fencing, acquired, missing, err := s.acquireMySQL(ctx, k, o, ttl)
		if !missing {
			return fencing, acquired, err
		}
	}
	// The cleanup kept deleting the key's idle row; report it held, as a lost
	// race, rather than retry without bound.
	return 0, false, nil
}

// acquireMySQL makes one MySQL acquisition: it creates the key's row on first
// use, then takes it over in a transaction. missing reports that the take
// found no row because the expired-row cleanup deleted it after it was
// created, so the acquisition must start over.
func (s *store) acquireMySQL(ctx context.Context, k, o any, ttl int64) (fencing int64, acquired, missing bool, err error) {
	attempts := ensureAttempts
	if _, ok := s.db.Tx(ctx); ok {
		attempts = 1
	}
	for range attempts {
		if _, err = s.db.ExecContext(ctx, s.stmts.ensure, k); err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return 0, false, false, err
	}
	tx, err := s.db.BeginScope(ctx, nil)
	if err != nil {
		return 0, false, false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, s.stmts.take, o, ttl, ttl, k)
	if err != nil {
		return 0, false, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, false, err
	}
	if n == 0 {
		if !s.cleanup {
			return 0, false, false, nil
		}
		var one int
		err = tx.QueryRowContext(ctx, s.stmts.exists, k).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, true, nil
		}
		return 0, false, false, err
	}
	if err := tx.QueryRowContext(ctx, s.stmts.fencing, k).Scan(&fencing); err != nil {
		return 0, false, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, false, err
	}
	return fencing, true, false, nil
}

// Read implements [leasing.Store].
func (s *store) Read(ctx context.Context, key string) (*providers.LockInfo, error) {
	return s.readLease(ctx, s.stmts.read, s.dialect.textArg(key))
}

// ReadOwned implements [leasing.Store].
func (s *store) ReadOwned(ctx context.Context, key, owner string, fencing uint64) (*providers.LockInfo, error) {
	return s.readLease(ctx, s.stmts.readOwned, s.dialect.textArg(key), s.dialect.textArg(owner), int64(fencing)) //nolint:gosec // see Acquire
}

// readLease returns the unexpired lease the query selects, or
// [providers.ErrLockNotHeld].
func (s *store) readLease(ctx context.Context, query string, args ...any) (*providers.LockInfo, error) {
	var (
		info                            providers.LockInfo
		fencing, acquired, renewed, ttl int64
	)
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&info.Key, &info.Owner, &fencing, &acquired, &renewed, &ttl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, providers.ErrLockNotHeld
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read SQL lock")
	}
	info.FencingToken = uint64(fencing) //nolint:gosec // fencing starts at 1 and only grows
	info.AcquiredAt = time.UnixMicro(acquired).UTC()
	info.LastRenewed = time.UnixMicro(renewed).UTC()
	info.TTL = time.Duration(ttl) * time.Microsecond
	info.IsStale = time.Since(info.LastRenewed) > info.TTL
	return &info, nil
}

// Renew implements [leasing.Store]. It locks the row first and runs the
// conditional renewal in the next statement of the same transaction, so the
// clock it compares against is read after any lock wait.
func (s *store) Renew(ctx context.Context, key, owner string, fencing uint64) (bool, error) {
	args := []any{s.ttl.Microseconds(), s.dialect.textArg(key), s.dialect.textArg(owner), int64(fencing)} //nolint:gosec // see Acquire
	tx, err := s.db.BeginScope(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.stmts.lockRow, s.dialect.textArg(key))
	if err != nil {
		return false, err
	}
	if err = rows.Close(); err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, s.stmts.renew, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, tx.Commit()
}

// Release implements [leasing.Store]: it ends the lease while it is still
// owner's (and fencing's, when known), keeping the row.
func (s *store) Release(ctx context.Context, key, owner string, fencing uint64) error {
	query, args := s.stmts.release, []any{s.dialect.textArg(key), s.dialect.textArg(owner), int64(fencing)} //nolint:gosec // see Acquire
	if fencing == 0 {
		query, args = s.stmts.releaseOwner, args[:2]
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return coreerrs.WrapOperation(err, "release SQL lock")
	}
	return nil
}

// Ping implements [leasing.Store].
func (s *store) Ping(ctx context.Context) error {
	return coreerrs.Wrap(s.db.PingContext(ctx), "sql database unreachable")
}
