// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// serverNow is the MongoDB aggregation variable holding the server's current
// time. Every lease decision uses it, so replicas with skewed clocks agree on
// whether a lease is still valid.
const serverNow = "$$NOW"

// errClosed reports an operation on a closed provider.
var errClosed = errors.New("provider is closed")

// lockDoc is the stored state of one lock key. The document outlives the
// leases taken on it: releasing a lock only ends its lease, so fencing keeps
// growing across holders instead of restarting when a document is removed.
type lockDoc struct {
	Key        string    `bson:"_id"`
	Owner      string    `bson:"owner"`
	Fencing    int64     `bson:"fencing"`
	AcquiredAt time.Time `bson:"acquired_at"`
	RenewedAt  time.Time `bson:"renewed_at"`
	ExpiresAt  time.Time `bson:"expires_at"`
	TTLMillis  int64     `bson:"ttl_ms"`
}

// Locker is a [providers.Provider] keeping each lock as a leased document in
// a MongoDB collection. A lock is taken by one conditional upsert that
// succeeds only while the key has no unexpired lease, renewed in the
// background, and released when its context ends or Release is called. Lease
// expiry is judged by the server clock. The fencing token grows by one with
// every acquisition of a key.
//
// The collection needs no index besides _id. Lock documents are kept after
// release (one per key ever locked) so fencing tokens stay monotonic. Every
// lock write uses majority write concern whatever the database handle
// carries: a lease acknowledged by a primary that then rolls it back in a
// failover would let a second holder receive the same fencing token. Lock
// decisions are made by those writes alone. Reads go to the primary whatever
// the handle's read preference, so GetLockInfo sees this client's own lease
// writes (a secondary may lag them); they keep the handle's read concern,
// since a majority read without a causally consistent session may not yet
// see such a write either.
type Locker struct {
	coll     *mongodrv.Collection
	opts     *options
	interval time.Duration // renewal interval

	mu     sync.Mutex // guards active, inflight registration, closeErrs and the closed transition
	active map[*lock]struct{}
	closed atomic.Bool
	// inflight counts Lock calls past the closed check; Close waits for them.
	inflight sync.WaitGroup

	// afterAcquire, when set, runs between a successful acquisition and its
	// registration; releaseFault, when set, can fail a release before it
	// reaches MongoDB. Test hooks.
	afterAcquire func()
	releaseFault func() error
	// acquireFault, when set, fails an acquisition after MongoDB applied it,
	// as a lost reply would; a test hook.
	acquireFault func() error

	// now is the monotonic clock lease deadlines are measured with; a field
	// so tests can make an acquisition arrive late.
	now func() time.Time
}

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
)

// New creates a MongoDB lock provider on db. It performs no I/O.
func New(db *mongodrv.Database, opts ...Option) (*Locker, error) {
	if db == nil {
		return nil, errors.New("mongo dlock: database is required")
	}
	o := newOptions(opts...)
	if math.IsNaN(o.renewRatio) || o.renewRatio <= 0 || o.renewRatio >= 1 {
		return nil, errors.New("mongo dlock: renew ratio must lie in (0, 1)")
	}
	// The lease is stored in whole milliseconds, so every deadline uses the
	// TTL truncated the same way; the renewal must come first.
	o.ttl = o.ttl.Truncate(time.Millisecond)
	if o.ttl < time.Millisecond {
		return nil, errors.New("mongo dlock: TTL must be at least 1ms")
	}
	interval := time.Duration(float64(o.ttl) * o.renewRatio)
	if interval < time.Millisecond || interval >= o.ttl {
		return nil, errors.New("mongo dlock: TTL x renew ratio must be at least 1ms and shorter than the TTL")
	}
	coll := db.Collection(o.collection, mongoopts.Collection().
		SetWriteConcern(writeconcern.Majority()).
		SetReadPreference(readpref.Primary()))
	return &Locker{
		coll:     coll,
		opts:     o,
		interval: interval,
		active:   make(map[*lock]struct{}),
		now:      time.Now,
	}, nil
}

// Lock makes a single attempt to take the lock for key and returns
// [errs.ErrLockNotHeld] when another holder has an unexpired lease on it.
//
// ctx scopes the lock: while it lives the lease is renewed every
// TTL × renew ratio; when it ends the lease is released. A lease that cannot
// be renewed because another holder took the key over is reported lost (see
// [providers.Lock.GetLockInfo]); one that fails transiently is retried on the
// next renewal.
func (l *Locker) Lock(ctx context.Context, key string) (providers.Lock, error) {
	// The lock outlives this call and must not join a caller's MongoDB
	// session: inside a transaction its writes would be rolled back with it
	// (reissuing a fencing token) and the renewal goroutine would share a
	// session that is not safe for concurrent use.
	ctx = isolated(ctx)
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
	doc, acquired, err := l.acquire(attemptCtx, key, owner)
	cancel()
	if err != nil {
		// The acquisition may have been applied even though no reply came
		// back; its owner id is unique to this attempt, so it is released by
		// owner (the fencing token is unknown). An unresolved release stays
		// registered for Close to retry. An attempt still executing on the
		// server after this cleanup lapses at its TTL.
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
		fencing:   doc.Fencing,
		stopRenew: stopRenew,
		done:      make(chan struct{}),
		guard:     make(chan struct{}, 1),
	}
	// Registration and Close share one lock: a lease acquired while Close
	// runs is either registered before Close collects the active locks, or
	// released here because the provider closed meanwhile.
	// A reply that arrives after the lease's deadline proves nothing: another
	// holder may own the key by now. Such an acquisition is released (a no-op
	// if it was taken over) and reported as not held.
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
// lease, creating the document on the key's first use. It is one atomic
// upsert matched on _id alone (MongoDB rejects $expr in an upsert filter);
// every field is set through $cond on whether the stored lease expired, so a
// held key is rewritten with its own values and the returned owner tells
// whether this caller won.
func (l *Locker) acquire(ctx context.Context, key, owner string) (lockDoc, bool, error) {
	ttl := l.opts.ttl.Milliseconds()
	free := bson.M{"$lte": bson.A{bson.M{"$ifNull": bson.A{"$expires_at", nil}}, serverNow}}
	take := func(taken, kept any) bson.M { return bson.M{"$cond": bson.A{free, taken, kept}} }
	update := mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"owner":       take(owner, "$owner"),
		"fencing":     take(bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$fencing", 0}}, 1}}, "$fencing"),
		"acquired_at": take(serverNow, "$acquired_at"),
		"renewed_at":  take(serverNow, "$renewed_at"),
		"expires_at":  take(bson.M{"$add": bson.A{serverNow, ttl}}, "$expires_at"),
		"ttl_ms":      take(ttl, "$ttl_ms"),
	}}}}
	var doc lockDoc
	err := l.coll.FindOneAndUpdate(ctx, bson.M{"_id": key}, update,
		mongoopts.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(mongoopts.After)).Decode(&doc)
	switch {
	case mongodrv.IsDuplicateKeyError(err):
		// Two first uses of the key raced to insert it; the other won.
		return lockDoc{}, false, nil
	case err != nil:
		return lockDoc{}, false, coreerrs.WrapOperation(err, "acquire MongoDB lock")
	case doc.Owner != owner:
		return lockDoc{}, false, nil
	}
	if l.acquireFault != nil {
		if err := l.acquireFault(); err != nil {
			return lockDoc{}, false, err
		}
	}
	return doc, true, nil
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
	return l.readLease(isolated(ctx), bson.M{"_id": key})
}

// readLease returns the unexpired lease matching filter, or
// [errs.ErrLockNotHeld].
func (l *Locker) readLease(ctx context.Context, filter bson.M) (*providers.LockInfo, error) {
	filter["$expr"] = bson.M{"$gt": bson.A{"$expires_at", serverNow}}
	var doc lockDoc
	err := l.coll.FindOne(ctx, filter).Decode(&doc)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return nil, errs.ErrLockNotHeld
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read MongoDB lock")
	}
	ttl := time.Duration(doc.TTLMillis) * time.Millisecond
	return &providers.LockInfo{
		Key:          doc.Key,
		Owner:        doc.Owner,
		AcquiredAt:   doc.AcquiredAt,
		LastRenewed:  doc.RenewedAt,
		TTL:          ttl,
		FencingToken: uint64(doc.Fencing), //nolint:gosec // fencing starts at 1 and only grows
		IsStale:      time.Since(doc.RenewedAt) > ttl,
	}, nil
}

// Close stops renewing every lock held through this provider, rejects
// further Lock calls, waits for acquisitions already in flight and releases
// every held lock. It returns the release failures; calling Close again
// retries the cleanup of what is still held (as does each lock's Release).
// The MongoDB client is managed by the caller.
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
// or the MongoDB deployment is unreachable.
func (l *Locker) Probe(ctx context.Context) error {
	if l.closed.Load() {
		return errClosed
	}
	// Lock writes need a primary: a reachable secondary must not count.
	if err := l.coll.Database().Client().Ping(ctx, readpref.Primary()); err != nil {
		return coreerrs.Wrap(err, "mongodb unreachable")
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
	fencing int64

	// stopRenew ends the renewal loop and cancels a renewal in flight; done
	// closes when the loop has returned.
	stopRenew context.CancelFunc
	done      chan struct{}

	// guard (a one-slot semaphore, so waiting honors a context) serializes
	// release attempts. ended records that the lease no longer needs
	// releasing: it was released, or another holder took it over.
	guard chan struct{}
	ended bool
}

var _ providers.Lock = (*lock)(nil)

// GetLockInfo returns this lock's own lease — its owner and the fencing
// token this acquisition received — while it is unexpired, and
// [errs.ErrLockNotHeld] once it was released, expired or taken over. It
// never reports another holder's lease, so the fencing token it returns is
// always this acquisition's.
func (lk *lock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	return lk.locker.readLease(isolated(ctx), lk.ownedFilter())
}

// Release stops renewing and ends the lease while this lock still holds it,
// within ctx bounded by the operations timeout. A failed release can be
// retried — by Release or by Close; releasing a lock that was already
// released or lost is a no-op.
func (lk *lock) Release(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(isolated(ctx), lk.locker.opts.operationsTimeout)
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
// or another holder takes the lease over. Each renewal is scheduled one
// renewal interval after the previous lease start (the acquisition or the
// last successful renewal request), so it always precedes that lease's
// deadline however late a reply arrived.
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
				// longer be trusted, so renewing stops. The server may still
				// hold the lease (a renewal applied but its reply lost), so
				// it is released conditionally; a failed release keeps the
				// lock registered for Release or Close to retry.
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
// whether renewing should continue: it stops once another holder took the
// lease over. A failed round trip is logged and retried later.
func (lk *lock) renew(ctx context.Context) (renewed, keep bool) {
	opCtx, cancel := context.WithTimeout(ctx, lk.locker.opts.operationsTimeout)
	defer cancel()
	ttl := lk.locker.opts.ttl.Milliseconds()
	// Only an unexpired lease is renewed: an expired one stays lost even
	// when nobody took it over.
	filter := lk.ownedFilter()
	filter["$expr"] = bson.M{"$gt": bson.A{"$expires_at", serverNow}}
	res, err := lk.locker.coll.UpdateOne(opCtx, filter, mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"renewed_at": serverNow,
		"expires_at": bson.M{"$add": bson.A{serverNow, ttl}},
	}}}})
	if err != nil {
		if ctx.Err() == nil {
			lk.locker.opts.logger.WarnContext(ctx, "failed to renew lock", slog.Any("error", err), slog.String("key", lk.key))
		}
		return false, true
	}
	if res.MatchedCount == 0 {
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
// owner and fencing filter make a retry harmless after a takeover.
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
	_, err := lk.locker.coll.UpdateOne(ctx, lk.ownedFilter(), mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"owner":      "",
		"expires_at": serverNow,
	}}}})
	if err != nil {
		return coreerrs.WrapOperation(err, "release MongoDB lock")
	}
	lk.ended = true
	lk.locker.untrack(lk)
	return nil
}

// ownedFilter matches the key only while this lock's lease is the latest one.
// The owner id is unique to one acquisition; the fencing token, when known,
// is matched as well.
func (lk *lock) ownedFilter() bson.M {
	filter := bson.M{"_id": lk.key, "owner": lk.owner}
	if lk.fencing != 0 {
		filter["fencing"] = lk.fencing
	}
	return filter
}

// isolated returns ctx with any MongoDB session it carries hidden, so lock
// operations never join a caller's session or transaction; cancellation,
// deadline and values are kept.
func isolated(ctx context.Context) context.Context {
	return mongodrv.NewSessionContext(ctx, nil)
}
