// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// errClosed reports an operation on a closed provider.
var errClosed = errors.New("provider is closed")

// ensureAttempts bounds the attempts of the MySQL statement that creates a
// key's row: InnoDB may resolve concurrent first uses of one key as a
// deadlock, which a retry settles.
const ensureAttempts = 3

// Locker is a [providers.Provider] keeping each lock as a leased row in a SQL
// table. A lock is taken by a conditional write that succeeds only while the
// key has no unexpired lease, renewed in the background, and released when
// its context ends or Release is called. Lease expiry is judged by the
// database clock, so replicas with skewed clocks agree. The fencing token
// grows by one with every acquisition of a key.
//
// Rows are kept after release (one per key ever locked) so fencing tokens
// stay monotonic; deleting rows, or restoring the table from a backup, can
// reissue tokens. The caller owns the *sql.DB; the locker never joins a
// caller's transaction.
type Locker struct {
	db        *sql.DB
	dialect   dialect
	table     string // quoted
	tableName string // unqualified, for the DDL lock
	stmts     statements
	opts      *options
	interval  time.Duration // renewal interval

	mu     sync.Mutex // guards active, inflight registration and the closed transition
	active map[*lock]struct{}
	closed atomic.Bool
	// inflight counts Lock calls past the closed check; Close waits for them.
	inflight sync.WaitGroup

	// afterAcquire, when set, runs between a successful acquisition and its
	// registration; releaseFault, when set, can fail a release before it
	// reaches the database; acquireFault, when set, fails an acquisition after
	// the database applied it, as a lost reply would. Test hooks.
	afterAcquire func()
	releaseFault func() error
	acquireFault func() error

	// now is the monotonic clock lease deadlines are measured with; a field
	// so tests can make an acquisition arrive late.
	now func() time.Time
}

// statements are the fixed queries, rendered once for the dialect.
type statements struct {
	acquire, ensure, take, fencing string
	read, readOwned                string
	renew, release, releaseOwner   string
}

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
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
	if math.IsNaN(o.renewRatio) || o.renewRatio <= 0 || o.renewRatio >= 1 {
		return nil, errors.New("sqldb dlock: renew ratio must lie in (0, 1)")
	}
	// Every deadline uses the TTL truncated to whole milliseconds, like the
	// MongoDB provider; the renewal must come first.
	o.ttl = o.ttl.Truncate(time.Millisecond)
	if o.ttl < time.Millisecond {
		return nil, errors.New("sqldb dlock: TTL must be at least 1ms")
	}
	interval := time.Duration(float64(o.ttl) * o.renewRatio)
	if interval < time.Millisecond || interval >= o.ttl {
		return nil, errors.New("sqldb dlock: TTL x renew ratio must be at least 1ms and shorter than the TTL")
	}
	l := &Locker{
		db:        db,
		dialect:   dl,
		tableName: sqldialect.Unqualified(o.tableName),
		opts:      o,
		interval:  interval,
		active:    make(map[*lock]struct{}),
		now:       time.Now,
	}
	if l.table, err = dl.Table(o.tableName); err != nil {
		return nil, err
	}
	l.stmts = l.buildStatements()
	return l, nil
}

func (l *Locker) buildStatements() statements {
	t, now, b := l.table, l.dialect.now, l.dialect.Bind
	const cols = "lock_key, owner, fencing, acquired_at, renewed_at, ttl_us"
	unexpired := " AND expires_at > " + now
	s := statements{
		read:      b("SELECT "+cols+" FROM "+t+" WHERE lock_key = ?"+unexpired, 1),
		readOwned: b("SELECT "+cols+" FROM "+t+" WHERE lock_key = ? AND owner = ? AND fencing = ?"+unexpired, 1),
		// Only an unexpired lease is renewed: an expired one stays lost even
		// when nobody took it over.
		renew: b("UPDATE "+t+" SET renewed_at = "+now+", expires_at = "+now+" + ?"+
			" WHERE lock_key = ? AND owner = ? AND fencing = ?"+unexpired, 1),
		release:      b("UPDATE "+t+" SET owner = '', expires_at = "+now+" WHERE lock_key = ? AND owner = ? AND fencing = ?", 1),
		releaseOwner: b("UPDATE "+t+" SET owner = '', expires_at = "+now+" WHERE lock_key = ? AND owner = ?", 1),
	}
	if l.dialect.name == DialectPostgres {
		// One statement: insert the key's row on first use, or take the lease
		// over while it is expired. ON CONFLICT waits for a concurrent writer
		// of the row and re-evaluates the condition after it; no row comes
		// back when the key is held.
		s.acquire = b("INSERT INTO "+t+" AS cur (lock_key, owner, fencing, acquired_at, renewed_at, expires_at, ttl_us)"+
			" VALUES (?, ?, 1, "+now+", "+now+", "+now+" + ?, ?)"+
			" ON CONFLICT (lock_key) DO UPDATE SET owner = EXCLUDED.owner, fencing = cur.fencing + 1,"+
			" acquired_at = "+now+", renewed_at = "+now+", expires_at = "+now+" + EXCLUDED.ttl_us, ttl_us = EXCLUDED.ttl_us"+
			" WHERE cur.expires_at <= "+now+
			" RETURNING fencing", 1)
		return s
	}
	// MySQL: create the row on first use, then take it over in a conditional
	// UPDATE (the fencing increment always changes the row, so the affected
	// count is exact) and read the new token in the same transaction.
	s.ensure = "INSERT INTO " + t + " (lock_key) VALUES (?) ON DUPLICATE KEY UPDATE lock_key = lock_key"
	s.take = "UPDATE " + t + " SET owner = ?, fencing = fencing + 1, acquired_at = " + now + ", renewed_at = " + now +
		", expires_at = " + now + " + ?, ttl_us = ? WHERE lock_key = ? AND expires_at <= " + now
	s.fencing = "SELECT fencing FROM " + t + " WHERE lock_key = ?"
	return s
}

// Lock makes a single attempt to take the lock for key and returns
// [errs.ErrLockNotHeld] when another holder has an unexpired lease on it.
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
	l.mu.Lock()
	if l.closed.Load() {
		l.mu.Unlock()
		return nil, errClosed
	}
	l.inflight.Add(1)
	l.mu.Unlock()
	defer l.inflight.Done()
	owner := uuid.NewString()

	// The lease cannot start before the request is sent, so it is valid at
	// most until start + TTL on this process's clock, however late the reply.
	start := l.now()
	attemptCtx, cancel := context.WithTimeout(ctx, l.opts.operationsTimeout)
	fencing, acquired, err := l.acquire(attemptCtx, key, owner)
	cancel()
	if err != nil {
		// The acquisition may have been applied even though no reply came
		// back (or its commit was); its owner id is unique to this attempt,
		// so it is released by owner. An unresolved release stays registered
		// for Close to retry.
		l.releaseAmbiguous(ctx, key, owner)
		l.opts.logger.ErrorContext(ctx, "failed to acquire lock", slog.Any("error", err), slog.String("key", key))
		return nil, err
	}
	if !acquired {
		return nil, errs.ErrLockNotHeld
	}

	renewCtx, stopRenew := context.WithCancel(ctx)
	lk := &lock{
		locker:    l,
		key:       key,
		owner:     owner,
		fencing:   fencing,
		stopRenew: stopRenew,
		done:      make(chan struct{}),
		guard:     make(chan struct{}, 1),
	}
	// Registration and Close share one lock: a lease acquired while Close
	// runs is either registered before Close collects the active locks, or
	// released here because the provider closed meanwhile. A reply that
	// arrives after the lease's deadline proves nothing: another holder may
	// own the key by now, so such an acquisition is released and reported as
	// not held.
	if l.afterAcquire != nil {
		l.afterAcquire()
	}
	late := l.now().Sub(start) >= l.opts.ttl
	l.mu.Lock()
	if late || l.closed.Load() {
		l.mu.Unlock()
		stopRenew()
		close(lk.done)
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.opts.operationsTimeout)
		defer cancel()
		if err := lk.endLease(releaseCtx); err != nil {
			// Keep it registered (renewal never started), so Close retries
			// the release instead of leaving the lease to its TTL.
			l.opts.logger.ErrorContext(ctx, "failed to release an unusable acquisition",
				slog.Any("error", err), slog.String("key", key))
			l.mu.Lock()
			l.active[lk] = struct{}{}
			l.mu.Unlock()
		}
		if late {
			return nil, errs.ErrLockNotHeld
		}
		return nil, errClosed
	}
	l.active[lk] = struct{}{}
	l.mu.Unlock()
	go lk.renewLoop(ctx, renewCtx, start)
	return lk, nil
}

// acquire takes the lease of key for owner when the key has no unexpired
// lease, creating the row on the key's first use, and returns the new fencing
// token.
func (l *Locker) acquire(ctx context.Context, key, owner string) (int64, bool, error) {
	fencing, acquired, err := l.acquireStmt(ctx, key, owner)
	if err != nil {
		return 0, false, coreerrs.WrapOperation(err, "acquire SQL lock")
	}
	if acquired && l.acquireFault != nil {
		if err := l.acquireFault(); err != nil {
			return 0, false, err
		}
	}
	return fencing, acquired, nil
}

func (l *Locker) acquireStmt(ctx context.Context, key, owner string) (int64, bool, error) {
	ttl := l.opts.ttl.Microseconds()
	k, o := l.dialect.textArg(key), l.dialect.textArg(owner)
	if l.dialect.name == DialectPostgres {
		var fencing int64
		err := l.db.QueryRowContext(ctx, l.stmts.acquire, k, o, ttl, ttl).Scan(&fencing)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return fencing, err == nil, err
	}

	var err error
	for range ensureAttempts {
		if _, err = l.db.ExecContext(ctx, l.stmts.ensure, k); err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return 0, false, err
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, l.stmts.take, o, ttl, ttl, k)
	if err != nil {
		return 0, false, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return 0, false, err
	}
	var fencing int64
	if err := tx.QueryRowContext(ctx, l.stmts.fencing, k).Scan(&fencing); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return fencing, true, nil
}

// releaseAmbiguous ends a lease an errored acquisition by owner may have
// taken, keeping it registered when the release fails.
func (l *Locker) releaseAmbiguous(ctx context.Context, key, owner string) {
	done := make(chan struct{})
	close(done)
	lk := &lock{locker: l, key: key, owner: owner, stopRenew: func() {}, done: done, guard: make(chan struct{}, 1)}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.opts.operationsTimeout)
	defer cancel()
	if err := lk.endLease(releaseCtx); err != nil {
		l.opts.logger.ErrorContext(ctx, "failed to release a possibly applied acquisition",
			slog.Any("error", err), slog.String("key", key))
		l.mu.Lock()
		l.active[lk] = struct{}{}
		l.mu.Unlock()
	}
}

// GetLockInfo returns the lease currently held on key, or
// [errs.ErrLockNotHeld] when the key has no unexpired lease.
func (l *Locker) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return l.readLease(ctx, l.stmts.read, l.dialect.textArg(key))
}

// readLease returns the unexpired lease the query selects, or
// [errs.ErrLockNotHeld].
func (l *Locker) readLease(ctx context.Context, query string, args ...any) (*providers.LockInfo, error) {
	var (
		info                            providers.LockInfo
		fencing, acquired, renewed, ttl int64
	)
	err := l.db.QueryRowContext(ctx, query, args...).Scan(&info.Key, &info.Owner, &fencing, &acquired, &renewed, &ttl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.ErrLockNotHeld
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

// Close stops renewing every lock held through this provider, rejects
// further Lock calls, waits for acquisitions already in flight and releases
// every held lock. It returns the release failures; calling Close again
// retries the cleanup of what is still held (as does each lock's Release).
// The *sql.DB is managed by the caller.
func (l *Locker) Close(ctx context.Context) error {
	l.mu.Lock()
	l.closed.Store(true)
	held := make([]*lock, 0, len(l.active))
	for lk := range l.active {
		held = append(held, lk)
	}
	l.mu.Unlock()
	// Renewal stops at once, so a Close that times out below cannot leave
	// a lease renewed forever; an unreleased lease then lapses after its TTL.
	for _, lk := range held {
		lk.stopRenew()
	}

	// Acquisitions already past the closed check either register before the
	// snapshot below or release themselves; wait until they have done so.
	drained := make(chan struct{})
	go func() {
		l.inflight.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for in-flight lock acquisitions")
	}

	l.mu.Lock()
	locks := make([]*lock, 0, len(l.active))
	for lk := range l.active {
		locks = append(locks, lk)
	}
	l.mu.Unlock()

	var errsOut []error
	for _, lk := range locks {
		if err := lk.Release(ctx); err != nil {
			l.opts.logger.ErrorContext(ctx, "failed to release lock on close",
				slog.Any("error", err), slog.String("key", lk.key))
			errsOut = append(errsOut, coreerrs.Wrapf(err, "release lock %q", lk.key))
		}
	}
	return errors.Join(errsOut...)
}

// Probe implements [providers.Prober]: it fails when the provider is closed
// or the database is unreachable.
func (l *Locker) Probe(ctx context.Context) error {
	if l.closed.Load() {
		return errClosed
	}
	if err := l.db.PingContext(ctx); err != nil {
		return coreerrs.Wrap(err, "sql database unreachable")
	}
	return nil
}

func (l *Locker) untrack(lk *lock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.active, lk)
}

// lock is one lease taken by [Locker.Lock].
type lock struct {
	locker  *Locker
	key     string
	owner   string
	fencing int64 // 0 when unknown (an ambiguous acquisition)

	// stopRenew ends the renewal loop and cancels a renewal in flight; done
	// closes when the loop has returned.
	stopRenew context.CancelFunc
	done      chan struct{}

	// guard (a one-slot semaphore, so waiting honors a context) serializes
	// release attempts. ended records that the lease no longer needs
	// releasing: it was released, or it expired or was taken over.
	guard chan struct{}
	ended bool
}

var _ providers.Lock = (*lock)(nil)

// GetLockInfo returns this lock's own lease — its owner and the fencing
// token this acquisition received — while it is unexpired, and
// [errs.ErrLockNotHeld] once it was released, expired or taken over.
func (lk *lock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	d := lk.locker.dialect
	return lk.locker.readLease(ctx, lk.locker.stmts.readOwned, d.textArg(lk.key), d.textArg(lk.owner), lk.fencing)
}

// Release stops renewing and ends the lease while this lock still holds it,
// within ctx bounded by the operations timeout. A failed release can be
// retried — by Release or by Close; releasing a lock that was already
// released or lost is a no-op.
func (lk *lock) Release(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, lk.locker.opts.operationsTimeout)
	defer cancel()
	lk.stopRenew()
	select {
	case <-lk.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return lk.endLease(ctx)
}

// renewLoop renews the lease until renewCtx ends — Release stops it, or the
// lock's context ended, in which case the loop releases the lease itself —
// or the lease is lost. Each renewal is scheduled one renewal interval after
// the previous lease start (the acquisition or the last successful renewal
// request), so it always precedes that lease's deadline however late a reply
// arrived.
func (lk *lock) renewLoop(lockCtx, renewCtx context.Context, leaseStart time.Time) {
	defer close(lk.done)
	timer := time.NewTimer(max(0, leaseStart.Add(lk.locker.interval).Sub(lk.locker.now())))
	defer timer.Stop()
	for {
		select {
		case <-renewCtx.Done():
			if lockCtx.Err() != nil {
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(lockCtx), lk.locker.opts.operationsTimeout)
				if err := lk.endLease(releaseCtx); err != nil {
					// The lock stays registered, so Close retries it.
					lk.locker.opts.logger.ErrorContext(releaseCtx, "failed to release lock after its context ended",
						slog.Any("error", err), slog.String("key", lk.key))
				}
				cancel()
			}
			return
		case <-timer.C:
			attempt := lk.locker.now()
			if attempt.Sub(leaseStart) >= lk.locker.opts.ttl {
				// No renewal is confirmed within the lease: ownership can no
				// longer be trusted, so renewing stops. The database may still
				// hold the lease (a renewal applied but its reply lost), so it
				// is released conditionally; a failed release keeps the lock
				// registered for Release or Close to retry.
				lk.locker.opts.logger.WarnContext(renewCtx, "lock lease not renewed in time; releasing it", slog.String("key", lk.key))
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(renewCtx), lk.locker.opts.operationsTimeout)
				if err := lk.endLease(releaseCtx); err != nil {
					lk.locker.opts.logger.ErrorContext(releaseCtx, "failed to release an unconfirmed lease",
						slog.Any("error", err), slog.String("key", lk.key))
				}
				cancel()
				return
			}
			renewed, keep := lk.renew(renewCtx)
			if !keep {
				return
			}
			if renewed {
				leaseStart = attempt
			}
			// After a failed attempt, retry one interval later; the lease
			// keeps its previous deadline meanwhile.
			next := leaseStart.Add(lk.locker.interval)
			if !renewed {
				next = attempt.Add(lk.locker.interval)
			}
			timer.Reset(max(0, next.Sub(lk.locker.now())))
		}
	}
}

// renew extends the lease. It reports whether the lease was extended and
// whether renewing should continue: it stops once the lease expired or was
// taken over. A failed round trip is logged and retried later.
func (lk *lock) renew(ctx context.Context) (renewed, keep bool) {
	opCtx, cancel := context.WithTimeout(ctx, lk.locker.opts.operationsTimeout)
	defer cancel()
	d := lk.locker.dialect
	res, err := lk.locker.db.ExecContext(opCtx, lk.locker.stmts.renew, lk.locker.opts.ttl.Microseconds(),
		d.textArg(lk.key), d.textArg(lk.owner), lk.fencing)
	var n int64
	if err == nil {
		n, err = res.RowsAffected()
	}
	if err != nil {
		if ctx.Err() == nil {
			lk.locker.opts.logger.WarnContext(ctx, "failed to renew lock", slog.Any("error", err), slog.String("key", lk.key))
		}
		return false, true
	}
	if n == 0 {
		lk.lose("lock lost: expired or taken over")
		return false, false
	}
	return true, true
}

// lose records that the lease ended without a release and unregisters the
// lock.
func (lk *lock) lose(msg string) {
	lk.locker.opts.logger.Warn(msg, slog.String("key", lk.key))
	lk.guard <- struct{}{}
	lk.ended = true
	<-lk.guard
	lk.locker.untrack(lk)
}

// endLease ends the lease unless it already ended, and unregisters the lock
// once it has. A failed update leaves the lock registered and retryable; the
// owner and fencing condition make a retry harmless after a takeover.
func (lk *lock) endLease(ctx context.Context) error {
	select {
	case lk.guard <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-lk.guard }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if lk.ended {
		lk.locker.untrack(lk)
		return nil
	}
	if fault := lk.locker.releaseFault; fault != nil {
		if err := fault(); err != nil {
			return err
		}
	}
	d := lk.locker.dialect
	query, args := lk.locker.stmts.release, []any{d.textArg(lk.key), d.textArg(lk.owner), lk.fencing}
	if lk.fencing == 0 {
		query, args = lk.locker.stmts.releaseOwner, args[:2]
	}
	if _, err := lk.locker.db.ExecContext(ctx, query, args...); err != nil {
		return coreerrs.WrapOperation(err, "release SQL lock")
	}
	lk.ended = true
	lk.locker.untrack(lk)
	return nil
}
