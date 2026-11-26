// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

// Common errors for lease operations.
var (
	ErrLeaseNotHeld = errors.New("lease not held")
	ErrLeaseExists  = errors.New("lease already exists")
)

const (
	// defaultReleaseTimeout is the timeout for releasing a lease during shutdown.
	defaultReleaseTimeout = 5 * time.Second

	// acquireCleanupTimeout bounds the extra time a failed acquisition can spend checking for and deleting an unconfirmed write.
	acquireCleanupTimeout = time.Second

	// DefaultRenewRatio is the fraction of the lease lifetime at which the
	// lease is renewed, and the fallback when a caller supplies a ratio
	// outside (0, 1].
	//
	// One third leaves three renewal attempts inside a lease lifetime, two of
	// them with room to spare, so one transient failure does not cost the
	// lease. It is the usual choice for lease keepalives — etcd sessions and
	// ZooKeeper heartbeats both renew at TTL/3.
	DefaultRenewRatio = 1.0 / 3.0

	// renewSafetyMarginRatio is the fraction of the lease lifetime the holder
	// gives up before the lease could expire on the server. It absorbs clock
	// rate drift between client and server and local scheduling delay, so the
	// holder stops believing it owns the lease before the server can hand the
	// key to someone else.
	renewSafetyMarginRatio = 0.1

	// minRenewRetryDelay is the shortest pause between renewal retries, and the
	// shortest per-attempt timeout. Once less than twice this is left before
	// the renewal deadline, the attempt in flight gets the rest of the window
	// and no further retry is scheduled.
	minRenewRetryDelay = 10 * time.Millisecond

	// renewWindowShare splits the time left before the renewal deadline: each
	// attempt, and each pause before the next one, gets 1/renewWindowShare of
	// it, so a retry always still fits.
	renewWindowShare = 2
)

// errRenewalDeadline reports that the lease could not be renewed before it may
// have expired on the server.
var errRenewalDeadline = errors.New("lease renewal deadline passed")

const (
	leaseIdle uint32 = iota
	leaseHeld
	leaseReleasePending
)

// LeaseCallbacks defines callbacks for lease state changes.
type LeaseCallbacks struct {
	// OnAcquired is called when the lease is successfully acquired.
	OnAcquired func()

	// OnLost is called after renewal has stopped and may call Release.
	OnLost func()

	// OnRenewed runs inline after a successful renewal. It must not call
	// Release, which waits for any in-flight renewal to finish.
	OnRenewed func()

	// OnReleased is called when the lease is voluntarily released.
	OnReleased func()
}

// LeaseObserver receives lease operation results and held-state transitions.
// Methods may be called concurrently.
type LeaseObserver interface {
	RecordOperation(op, result string)
	SetHeld(held bool)
}

// Lease represents a distributed lease that can be acquired, renewed, and released.
type Lease struct {
	ops  *KVOps
	key  string
	ttl  time.Duration
	opts *leaseOptions

	state atomic.Uint32
	// value holds the lease payload (owner identifier) as an atomically
	// swappable pointer. The camping goroutine reads it on every Renew
	// while UpdateValue may be called concurrently from a callback
	// goroutine (e.g. dlock.OnRenewed) — using atomic.Pointer keeps
	// reads and writes race-free without locking the hot Renew path.
	value atomic.Pointer[[]byte]

	// cancelMu guards renewal and caller-context cleanup.
	cancelMu    sync.Mutex
	cancel      context.CancelFunc
	campDone    <-chan struct{}
	stopCleanup func() bool

	// releaseGuard (a one-slot semaphore, so waiting honors a context)
	// serializes explicit and context-scoped Release calls.
	releaseGuard chan struct{}

	logger *slog.Logger
}

// NewLease creates a new Lease instance.
//
// key identifies the lease in the KV store, ttl is its lifetime and value is
// the payload stored with it (the owner identifier).
func NewLease(kv jetstream.KeyValue, key string, ttl time.Duration, value []byte, opts ...LeaseOption) *Lease {
	o := newLeaseOptions(opts...)

	l := &Lease{
		ops:    NewKVOps(kv, o.logger),
		key:    key,
		ttl:    ttl,
		opts:   o,
		logger: o.logger.With(slog.String("key", key)),

		releaseGuard: make(chan struct{}, 1),
	}
	// Seed the race-safe value from the initial value. Take a defensive
	// copy so a caller who mutates the slice they passed in does not
	// race with the renew goroutine reading it.
	initial := append([]byte(nil), value...)
	l.value.Store(&initial)
	return l
}

// currentValue returns the most-recently-set lease payload (initial
// value passed to [NewLease], or whatever the latest UpdateValue installed).
// The returned slice MUST NOT be mutated by callers — it is shared with
// concurrent reads.
func (l *Lease) currentValue() []byte {
	if p := l.value.Load(); p != nil {
		return *p
	}
	return nil
}

// Acquire attempts to acquire the lease.
// Returns true if successfully acquired, false if already held by another owner.
func (l *Lease) Acquire(ctx context.Context) (bool, error) {
	return l.acquireAt(ctx, time.Now())
}

func (l *Lease) acquireAt(ctx context.Context, started time.Time) (bool, error) {
	revision, err := l.tryAcquire(ctx)
	if err == nil && revision != 0 && time.Since(started) >= l.ttl-l.safetyMargin() {
		err = ErrLeaseNotHeld
	}
	if err != nil {
		l.record("acquire", err)
		// A write may have landed even if its acknowledgment did not.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acquireCleanupTimeout)
		defer cancel()
		if releaseErr := l.deleteOwned(releaseCtx, revision); releaseErr != nil {
			l.logger.WarnContext(releaseCtx, "failed to release an unconfirmed acquisition", slog.Any("error", releaseErr))
		}
		return false, err
	}
	if revision != 0 {
		l.record("acquire", nil)
		l.setState(leaseHeld)
		if l.opts.callbacks.OnAcquired != nil {
			l.opts.callbacks.OnAcquired()
		}
	}
	return revision != 0, nil
}

func (l *Lease) tryAcquire(ctx context.Context) (uint64, error) {
	// Try to create the key - only succeeds if it doesn't exist
	revision, err := l.ops.Create(ctx, l.key, l.currentValue())
	if err == nil {
		return revision, nil
	}

	// If key exists, check if we're the owner or if it's stale
	if errors.Is(err, jetstream.ErrKeyExists) {
		return l.refreshOwned(ctx)
	}

	return 0, err
}

// refreshOwned renews a key already held by this owner.
func (l *Lease) refreshOwned(ctx context.Context) (uint64, error) {
	entry, err := l.ops.Get(ctx, l.key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return 0, nil
		}
		return 0, err
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil {
		return 0, nil
	}

	// Check if we're already the owner
	if l.owns(entry.Value()) {
		// We own it, just update to renew
		revision, err := l.ops.Update(ctx, l.key, l.currentValue(), entry.Revision())
		if errors.Is(err, jetstream.ErrKeyExists) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		return revision, nil
	}

	// Someone else owns it
	return 0, nil
}

func (l *Lease) setState(state uint32) {
	l.state.Store(state)
	if l.opts.observer != nil {
		l.opts.observer.SetHeld(state == leaseHeld)
	}
}

// record counts one lease operation, classifying it by whether it failed.
func (l *Lease) record(op string, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	if l.opts.observer != nil {
		l.opts.observer.RecordOperation(op, result)
	}
}

// lost marks the lease as no longer held and reports it as a failed renewal.
// Every way of discovering the loss — the key gone, a tombstone, another owner,
// a revision that moved on — leaves the same state behind.
func (l *Lease) lost() (bool, error) {
	l.setState(leaseIdle)
	l.record("renew", ErrLeaseNotHeld)

	return false, ErrLeaseNotHeld
}

// Renew attempts to renew the lease.
// Returns true if successfully renewed, false if lease was lost.
func (l *Lease) Renew(ctx context.Context) (bool, error) {
	entry, err := l.ops.Get(ctx, l.key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return l.lost()
		}
		l.record("renew", err)

		return false, err
	}

	// A nil entry or a tombstone reads as the key being gone.
	if entry == nil || entry.Value() == nil {
		return l.lost()
	}

	// Someone else took the key over.
	if !l.owns(entry.Value()) {
		return l.lost()
	}

	// Rewrite the key at the revision we just read, which resets its TTL. The
	// revision makes this a compare-and-set: a mismatch means the key moved on
	// without us, so the lease is gone rather than merely unwritable.
	if _, err = l.ops.Update(ctx, l.key, l.currentValue(), entry.Revision()); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return l.lost()
		}
		l.record("renew", err)

		return false, err
	}

	l.record("renew", nil)

	if l.opts.callbacks.OnRenewed != nil {
		l.opts.callbacks.OnRenewed()
	}

	return true, nil
}

// Release voluntarily releases the lease and stops renewing it.
//
// It is idempotent: the deferred release in a caller's critical section, a
// provider shutting down, and the renewal loop's own teardown can all reach the
// same lease, and exactly one of them performs the delete.
// Concurrent calls are serialized, so none returns while another is still
// releasing. A failed release stays pending for a later Release to retry.
func (l *Lease) Release(ctx context.Context) error {
	l.StopCamping()
	l.cancelMu.Lock()
	done := l.campDone
	l.cancelMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case l.releaseGuard <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-l.releaseGuard }()

	if l.state.Load() == leaseIdle {
		l.stopContextRelease()
		return nil
	}

	l.setState(leaseReleasePending)
	err := l.deleteOwned(ctx, 0)
	l.record("release", err)
	if err != nil {
		return err
	}
	l.setState(leaseIdle)
	l.stopContextRelease()
	if l.opts.callbacks.OnReleased != nil {
		l.opts.callbacks.OnReleased()
	}
	return nil
}

func (l *Lease) owns(value []byte) bool {
	if l.opts.ownerCheck != nil {
		return l.opts.ownerCheck(value)
	}
	return bytes.Equal(value, l.currentValue())
}

func (l *Lease) deleteOwned(ctx context.Context, revision uint64) error {
	entry, err := l.ops.Get(ctx, l.key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return err
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil || !l.owns(entry.Value()) {
		return nil
	}
	if revision != 0 && entry.Revision() != revision {
		return nil
	}

	// Revision-checked: between the Get above and this delete the key can
	// expire and be re-acquired by another holder, and an unconditional delete
	// would drop *their* lock. The revision makes the delete a compare-and-set.
	err = l.ops.DeleteWithRevision(ctx, l.key, entry.Revision())
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	return err
}

// ReleasePending reports that ownership is uncertain and release can be retried.
func (l *Lease) ReleasePending() bool {
	return l.state.Load() == leaseReleasePending
}

// IsHeld returns true if the lease is currently held.
func (l *Lease) IsHeld() bool {
	return l.state.Load() == leaseHeld
}

// GetOps returns the underlying KVOps instance.
func (l *Lease) GetOps() *KVOps {
	return l.ops
}

// UpdateValue updates the value stored with the lease.
// The new value will be used in subsequent renewals.
//
// Safe to call concurrently with the renew goroutine — UpdateValue
// atomically swaps an internal pointer rather than mutating the existing
// slice. A defensive copy is taken so the caller may safely mutate or
// recycle the supplied buffer after the call returns.
func (l *Lease) UpdateValue(val []byte) {
	cp := append([]byte(nil), val...)
	l.value.Store(&cp)
}

// RunCamping acquires the lease and starts renewing it in the background.
// Returns true if the lease was acquired, false otherwise.
//
// ctx scopes the lease: cancel it and the renewal stops and the lease is
// released. acquireTimeout bounds only the acquisition attempt, on a context
// of its own — folding it into ctx would make the lease expire on the acquire
// deadline and drop it while its holder was still inside the critical section.
//
// The renewal also ends on [Lease.Release] or [Lease.StopCamping].
func (l *Lease) RunCamping(ctx context.Context, acquireTimeout time.Duration) (bool, error) {
	// Taken before the request is sent: the server writes the key, and starts
	// its TTL, only after that. Use this instant as the conservative start.
	acquiredAt := time.Now()

	acquireCtx, cancelAcquire := corectx.WithMaxTimeout(ctx, acquireTimeout)
	acquired, err := l.acquireAt(acquireCtx, acquiredAt)
	cancelAcquire()

	if err != nil {
		return false, err
	}

	if !acquired {
		return false, nil
	}

	campCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	l.cancelMu.Lock()
	l.cancel = cancel
	l.campDone = done
	l.stopCleanup = context.AfterFunc(ctx, func() {
		defer panics.Handle(ctx)
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultReleaseTimeout)
		defer cancel()
		if err := l.Release(releaseCtx); err != nil {
			l.logger.ErrorContext(releaseCtx, "failed to release lease", slog.Any("error", err))
		}
	})
	l.cancelMu.Unlock()

	go func() {
		defer panics.Handle(campCtx)
		defer cancel()
		lost := func() bool {
			defer close(done)
			return l.campingLoop(campCtx, acquiredAt)
		}()
		if lost && !l.ReleasePending() {
			l.stopContextRelease()
		}
		// Publish renewal completion before notifying OnLost, so the
		// callback can release without waiting for itself.
		if lost && l.opts.callbacks.OnLost != nil {
			l.opts.callbacks.OnLost()
		}
	}()

	return true, nil
}

func (l *Lease) stopContextRelease() {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	if l.stopCleanup != nil {
		l.stopCleanup()
		l.stopCleanup = nil
	}
}

// StopCamping stops the renewal loop and leaves the lease eligible for cleanup
// by Release or by cancellation of the context passed to RunCamping.
func (l *Lease) StopCamping() {
	l.cancelMu.Lock()
	cancel := l.cancel
	l.cancel = nil
	l.cancelMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// RenewInterval derives the renewal tick from a lease lifetime and a ratio.
//
// The ratio decides how many renewal attempts fall inside one lease lifetime —
// floor(1/ratio) — which is the budget for surviving a transient failure
// without dropping the lease. It is clamped into (0, 1]: a non-positive
// interval makes time.NewTicker panic, and the renewal goroutine's recover
// would swallow that, leaving a lease that is never renewed and silently
// expires.
func RenewInterval(ttl time.Duration, ratio float64) time.Duration {
	if ratio <= 0 || ratio > 1 || math.IsNaN(ratio) {
		ratio = DefaultRenewRatio
	}

	return time.Duration(float64(ttl) * ratio)
}

// safetyMargin is how long before the lease could expire the holder stops
// trusting it.
func (l *Lease) safetyMargin() time.Duration {
	return time.Duration(float64(l.ttl) * renewSafetyMarginRatio)
}

// renewEvery is the camping loop's renewal cadence: [RenewInterval], clamped so
// a renewal always starts at least one safety margin before the renewal
// deadline. A ratio close to 1 would otherwise schedule the renewal at, or
// past, the point the lease may already have expired.
func (l *Lease) renewEvery() time.Duration {
	return min(RenewInterval(l.ttl, l.opts.renewRatio), l.ttl-2*l.safetyMargin())
}

// renewRetryable reports whether a failed renewal is worth retrying while the
// lease may still be valid. Losing the key — gone, taken over, or its revision
// moved on — is definitive, and so is a connection its owner has closed.
// Anything else, a timed-out attempt included, may clear up on its own.
func renewRetryable(err error) bool {
	return !errors.Is(err, ErrLeaseNotHeld) && !errors.Is(err, nats.ErrConnectionClosed)
}

// renewWithRetry renews a lease that is valid until validUntil, retrying
// failures with backoff for as long as the lease may still be valid. It returns
// the time the successful attempt started: the server rewrote the key after
// that, so the renewed lease is valid at least until then plus the TTL.
//
// Every attempt is bounded: the whole retry runs under a deadline one safety
// margin before validUntil, and each attempt gets half of the time left, so a
// request that is never answered cannot use up the window that a fresh attempt
// would need once the server recovers.
func (l *Lease) renewWithRetry(ctx context.Context, validUntil time.Time, every time.Duration) (time.Time, error) {
	deadline := validUntil.Add(-l.safetyMargin())
	if time.Until(deadline) <= 0 {
		return time.Time{}, errRenewalDeadline
	}

	renewCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Half of what is left, so a retry always fits before the deadline. Zero
	// once too little is left, which ends the retry loop.
	halfLeft := func() time.Duration {
		if left := time.Until(deadline) / renewWindowShare; left >= minRenewRetryDelay {
			return left
		}
		return 0
	}

	backoff := coreretry.Exponential(
		min(retryBaseDelay, every),
		coreretry.WithExponentialMaxDelay(every),
		coreretry.WithExponentialJitter(DefaultJitter),
	)

	var renewedAt time.Time

	err := coreretry.Do(renewCtx, func(ctx context.Context) error {
		// Too little left to split: the attempt gets all of it, bounded by
		// the deadline of ctx.
		attemptCtx, cancelAttempt := ctx, context.CancelFunc(func() {})
		if left := halfLeft(); left > 0 {
			attemptCtx, cancelAttempt = context.WithTimeout(ctx, left)
		}
		defer cancelAttempt()

		start := time.Now()
		if _, err := l.Renew(attemptCtx); err != nil {
			return err
		}
		renewedAt = start

		return nil
	},
		coreretry.WithMaxAttempts(-1),
		coreretry.WithShouldRetry(renewRetryable),
		coreretry.WithNextDelay(func(attempt int, err error) time.Duration {
			if left := halfLeft(); left > 0 {
				return min(backoff(attempt, err), left)
			}
			return 0
		}),
		coreretry.WithOnRetry(func(attempt int, err error, delay time.Duration) {
			l.logger.WarnContext(ctx, "failed to renew lease, retrying",
				slog.Int("attempt", attempt+1), slog.Duration("delay", delay), slog.Any("error", err))
		}),
	)
	if err != nil && ctx.Err() == nil && renewCtx.Err() != nil {
		err = fmt.Errorf("%w: %w", errRenewalDeadline, err)
	}

	return renewedAt, err
}

// campingLoop maintains the lease by periodically renewing it.
//
// A failed renewal is retried for as long as the lease may still be valid —
// last successful write plus TTL, less a safety margin. The lease is reported
// lost when that window closes, or at once when the failure is definitive.
func (l *Lease) campingLoop(ctx context.Context, acquiredAt time.Time) (lost bool) {
	every := l.renewEvery()
	writtenAt := acquiredAt

	timer := time.NewTimer(time.Until(writtenAt.Add(every)))
	defer timer.Stop()

	l.logger.DebugContext(ctx, "camping loop started", slog.Duration("renew_interval", every))
	defer l.logger.DebugContext(ctx, "camping loop stopped")

	for {
		select {
		case <-timer.C:
			if ctx.Err() != nil {
				return false
			}
			renewedAt, err := l.renewWithRetry(ctx, writtenAt.Add(l.ttl), every)
			if err == nil {
				writtenAt = renewedAt
				timer.Reset(time.Until(writtenAt.Add(every)))
				l.logger.DebugContext(ctx, "lease renewed")

				continue
			}

			// Renew marks definitive loss. Otherwise, an unanswered write
			// may still own the key, so leave cleanup pending.
			if !errors.Is(err, ErrLeaseNotHeld) {
				if ctx.Err() != nil {
					return false
				}
				l.setState(leaseReleasePending)
			}
			l.logger.WarnContext(ctx, "lease lost during renewal", slog.Any("error", err))
			return true

		case <-ctx.Done():
			return false
		}
	}
}
