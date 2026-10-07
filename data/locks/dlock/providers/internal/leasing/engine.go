// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrClosed reports an operation on a closed provider.
var ErrClosed = errors.New("provider is closed")

// Store is the storage side of a lease provider. Every lease decision it makes
// compares against the storage's clock, so replicas with skewed clocks agree.
type Store interface {
	// Acquire takes the lease of key for owner when the key has no unexpired
	// lease, and returns the new fencing token; acquired is false when another
	// holder has an unexpired lease. The token grows by one with every
	// acquisition of a key.
	Acquire(ctx context.Context, key, owner string) (fencing uint64, acquired bool, err error)
	// Read returns the unexpired lease of key, or [errs.ErrLockNotHeld].
	Read(ctx context.Context, key string) (*providers.LockInfo, error)
	// ReadOwned returns the lease of key while it is unexpired and still the
	// one owner took with fencing, or [errs.ErrLockNotHeld].
	ReadOwned(ctx context.Context, key, owner string, fencing uint64) (*providers.LockInfo, error)
	// Renew extends the lease while it is unexpired and still the one owner
	// took with fencing, judging expiry by a clock read after any wait for the
	// record; renewed is false once it expired or was taken over.
	Renew(ctx context.Context, key, owner string, fencing uint64) (renewed bool, err error)
	// Release ends the lease while it is still owner's — and fencing's, when
	// fencing is not 0 (an acquisition whose reply was lost) — keeping the
	// record so tokens never restart. Releasing a lease that is gone is not
	// an error.
	Release(ctx context.Context, key, owner string, fencing uint64) error
	// Ping reports whether the storage is reachable.
	Ping(ctx context.Context) error
}

// Config holds the engine's timing and logging.
type Config struct {
	Logger *slog.Logger
	// TTL is the lease length; Interval the renewal interval, shorter than it.
	TTL, Interval time.Duration
	// OperationsTimeout bounds one acquisition attempt, renewal or release.
	OperationsTimeout time.Duration
}

// Timing validates a TTL and renew ratio for the provider named name and
// returns the TTL truncated to whole milliseconds and the renewal interval.
func Timing(name string, ttl time.Duration, renewRatio float64) (time.Duration, time.Duration, error) {
	if math.IsNaN(renewRatio) || renewRatio <= 0 || renewRatio >= 1 {
		return 0, 0, fmt.Errorf("%s: renew ratio must lie in (0, 1)", name)
	}
	ttl = ttl.Truncate(time.Millisecond)
	if ttl < time.Millisecond {
		return 0, 0, fmt.Errorf("%s: TTL must be at least 1ms", name)
	}
	interval := time.Duration(float64(ttl) * renewRatio)
	if interval < time.Millisecond || interval >= ttl {
		return 0, 0, fmt.Errorf("%s: TTL x renew ratio must be at least 1ms and shorter than the TTL", name)
	}
	return ttl, interval, nil
}

// Engine runs the lease lifecycle over a [Store]; it implements the
// [providers.Provider] methods its provider exposes.
type Engine struct {
	store Store
	cfg   Config

	mu     sync.Mutex // guards active, inflight registration and the closed transition
	active map[*lock]struct{}
	closed atomic.Bool
	// inflight counts Lock calls past the closed check; Close waits for them.
	inflight sync.WaitGroup

	// AfterAcquire, when set, runs between a successful acquisition and its
	// registration; ReleaseFault, when set, can fail a release before it
	// reaches the store; AcquireFault, when set, fails an acquisition after
	// the store applied it, as a lost reply would. Test hooks.
	AfterAcquire func()
	ReleaseFault func() error
	AcquireFault func() error

	// Now is the monotonic clock lease deadlines are measured with; a field
	// so tests can make an acquisition arrive late.
	Now func() time.Time
}

// New returns an engine over store.
func New(store Store, cfg Config) *Engine {
	return &Engine{store: store, cfg: cfg, active: make(map[*lock]struct{}), Now: time.Now}
}

// Interval returns the renewal interval.
func (e *Engine) Interval() time.Duration { return e.cfg.Interval }

// Lock makes a single attempt to take the lock for key and returns
// [errs.ErrLockNotHeld] when another holder has an unexpired lease on it.
//
// ctx scopes the lock: while it lives the lease is renewed every renewal
// interval; when it ends the lease is released. A lease that cannot be
// renewed because it expired or another holder took the key over is reported
// lost; one that fails transiently is retried on the next renewal.
func (e *Engine) Lock(ctx context.Context, key string) (providers.Lock, error) {
	e.mu.Lock()
	if e.closed.Load() {
		e.mu.Unlock()
		return nil, ErrClosed
	}
	e.inflight.Add(1)
	e.mu.Unlock()
	defer e.inflight.Done()
	owner := uuid.NewString()

	// The lease cannot start before the request is sent, so it is valid at
	// most until start + TTL on this process's clock, however late the reply.
	start := e.Now()
	attemptCtx, cancel := context.WithTimeout(ctx, e.cfg.OperationsTimeout)
	fencing, acquired, err := e.acquire(attemptCtx, key, owner)
	cancel()
	if err != nil {
		// The acquisition may have been applied even though no reply came
		// back; its owner id is unique to this attempt, so it is released by
		// owner. An unresolved release stays registered for Close to retry.
		e.releaseAmbiguous(ctx, key, owner)
		e.cfg.Logger.ErrorContext(ctx, "failed to acquire lock", slog.Any("error", err), slog.String("key", key))
		return nil, err
	}
	if !acquired {
		return nil, errs.ErrLockNotHeld
	}

	renewCtx, stopRenew := context.WithCancel(ctx)
	lk := &lock{
		engine:    e,
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
	if e.AfterAcquire != nil {
		e.AfterAcquire()
	}
	late := e.Now().Sub(start) >= e.cfg.TTL
	e.mu.Lock()
	if late || e.closed.Load() {
		e.mu.Unlock()
		stopRenew()
		close(lk.done)
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.cfg.OperationsTimeout)
		defer cancel()
		if err := lk.endLease(releaseCtx); err != nil {
			// Keep it registered (renewal never started), so Close retries
			// the release instead of leaving the lease to its TTL.
			e.cfg.Logger.ErrorContext(ctx, "failed to release an unusable acquisition",
				slog.Any("error", err), slog.String("key", key))
			e.track(lk)
		}
		if late {
			return nil, errs.ErrLockNotHeld
		}
		return nil, ErrClosed
	}
	e.active[lk] = struct{}{}
	e.mu.Unlock()
	go lk.renewLoop(ctx, renewCtx, start)
	return lk, nil
}

// acquire runs the store's acquisition and the acquisition fault hook.
func (e *Engine) acquire(ctx context.Context, key, owner string) (uint64, bool, error) {
	fencing, acquired, err := e.store.Acquire(ctx, key, owner)
	if err != nil || !acquired {
		return 0, false, err
	}
	if e.AcquireFault != nil {
		if err := e.AcquireFault(); err != nil {
			return 0, false, err
		}
	}
	return fencing, true, nil
}

// releaseAmbiguous ends a lease an errored acquisition by owner may have
// taken, keeping it registered when the release fails.
func (e *Engine) releaseAmbiguous(ctx context.Context, key, owner string) {
	done := make(chan struct{})
	close(done)
	lk := &lock{engine: e, key: key, owner: owner, stopRenew: func() {}, done: done, guard: make(chan struct{}, 1)}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.cfg.OperationsTimeout)
	defer cancel()
	if err := lk.endLease(releaseCtx); err != nil {
		e.cfg.Logger.ErrorContext(ctx, "failed to release a possibly applied acquisition",
			slog.Any("error", err), slog.String("key", key))
		e.track(lk)
	}
}

// GetLockInfo returns the lease currently held on key, or
// [errs.ErrLockNotHeld] when the key has no unexpired lease.
func (e *Engine) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return e.store.Read(ctx, key)
}

// Close stops renewing every lock held through the engine, rejects further
// Lock calls, waits for acquisitions already in flight and releases every
// held lock. It returns the release failures; calling Close again retries the
// cleanup of what is still held (as does each lock's Release).
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	e.closed.Store(true)
	held := e.snapshot()
	e.mu.Unlock()
	// Renewal stops at once, so a Close that times out below cannot leave
	// a lease renewed forever; an unreleased lease then lapses after its TTL.
	for _, lk := range held {
		lk.stopRenew()
	}

	// Acquisitions already past the closed check either register before the
	// snapshot below or release themselves; wait until they have done so.
	drained := make(chan struct{})
	go func() {
		e.inflight.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for in-flight lock acquisitions")
	}

	e.mu.Lock()
	locks := e.snapshot()
	e.mu.Unlock()

	var errsOut []error
	for _, lk := range locks {
		if err := lk.Release(ctx); err != nil {
			e.cfg.Logger.ErrorContext(ctx, "failed to release lock on close",
				slog.Any("error", err), slog.String("key", lk.key))
			errsOut = append(errsOut, coreerrs.Wrapf(err, "release lock %q", lk.key))
		}
	}
	return errors.Join(errsOut...)
}

// Probe fails when the engine is closed or the store is unreachable.
func (e *Engine) Probe(ctx context.Context) error {
	if e.closed.Load() {
		return ErrClosed
	}
	return e.store.Ping(ctx)
}

// snapshot returns the active locks; the caller holds mu.
func (e *Engine) snapshot() []*lock {
	locks := make([]*lock, 0, len(e.active))
	for lk := range e.active {
		locks = append(locks, lk)
	}
	return locks
}

func (e *Engine) track(lk *lock) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active[lk] = struct{}{}
}

func (e *Engine) untrack(lk *lock) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, lk)
}

// lock is one lease taken by [Engine.Lock].
type lock struct {
	engine  *Engine
	key     string
	owner   string
	fencing uint64 // 0 when unknown (an ambiguous acquisition)

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
// [errs.ErrLockNotHeld] once it was released, expired or taken over. It
// never reports another holder's lease.
func (lk *lock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	return lk.engine.store.ReadOwned(ctx, lk.key, lk.owner, lk.fencing)
}

// Release stops renewing and ends the lease while this lock still holds it,
// within ctx bounded by the operations timeout. A failed release can be
// retried — by Release or by Close; releasing a lock that was already
// released or lost is a no-op.
func (lk *lock) Release(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, lk.engine.cfg.OperationsTimeout)
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
	e := lk.engine
	defer close(lk.done)
	timer := time.NewTimer(max(0, leaseStart.Add(e.cfg.Interval).Sub(e.Now())))
	defer timer.Stop()
	for {
		select {
		case <-renewCtx.Done():
			if lockCtx.Err() != nil {
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(lockCtx), e.cfg.OperationsTimeout)
				if err := lk.endLease(releaseCtx); err != nil {
					// The lock stays registered, so Close retries it.
					e.cfg.Logger.ErrorContext(releaseCtx, "failed to release lock after its context ended",
						slog.Any("error", err), slog.String("key", lk.key))
				}
				cancel()
			}
			return
		case <-timer.C:
			attempt := e.Now()
			if attempt.Sub(leaseStart) >= e.cfg.TTL {
				// No renewal is confirmed within the lease: ownership can no
				// longer be trusted, so renewing stops. The store may still
				// hold the lease (a renewal applied but its reply lost), so it
				// is released conditionally; a failed release keeps the lock
				// registered for Release or Close to retry.
				e.cfg.Logger.WarnContext(renewCtx, "lock lease not renewed in time; releasing it", slog.String("key", lk.key))
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(renewCtx), e.cfg.OperationsTimeout)
				if err := lk.endLease(releaseCtx); err != nil {
					e.cfg.Logger.ErrorContext(releaseCtx, "failed to release an unconfirmed lease",
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
			next := leaseStart.Add(e.cfg.Interval)
			if !renewed {
				next = attempt.Add(e.cfg.Interval)
			}
			timer.Reset(max(0, next.Sub(e.Now())))
		}
	}
}

// renew extends the lease. It reports whether the lease was extended and
// whether renewing should continue: it stops once the lease expired or was
// taken over. A failed round trip is logged and retried later.
func (lk *lock) renew(ctx context.Context) (renewed, keep bool) {
	opCtx, cancel := context.WithTimeout(ctx, lk.engine.cfg.OperationsTimeout)
	defer cancel()
	renewed, err := lk.engine.store.Renew(opCtx, lk.key, lk.owner, lk.fencing)
	if err != nil {
		if ctx.Err() == nil {
			lk.engine.cfg.Logger.WarnContext(ctx, "failed to renew lock", slog.Any("error", err), slog.String("key", lk.key))
		}
		return false, true
	}
	if !renewed {
		lk.lose("lock lost: expired or taken over")
		return false, false
	}
	return true, true
}

// lose records that the lease ended without a release and unregisters the
// lock.
func (lk *lock) lose(msg string) {
	lk.engine.cfg.Logger.Warn(msg, slog.String("key", lk.key))
	lk.guard <- struct{}{}
	lk.ended = true
	<-lk.guard
	lk.engine.untrack(lk)
}

// endLease ends the lease unless it already ended, and unregisters the lock
// once it has. A failed release leaves the lock registered and retryable; the
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
		lk.engine.untrack(lk)
		return nil
	}
	if fault := lk.engine.ReleaseFault; fault != nil {
		if err := fault(); err != nil {
			return err
		}
	}
	if err := lk.engine.store.Release(ctx, lk.key, lk.owner, lk.fencing); err != nil {
		return err
	}
	lk.ended = true
	lk.engine.untrack(lk)
	return nil
}
