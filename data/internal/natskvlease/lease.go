// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"cmp"
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

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

// Common errors for lease operations.
var (
	ErrLeaseNotHeld = errors.New("lease not held")
	ErrLeaseExists  = errors.New("lease already exists")
)

const (
	// defaultReleaseTimeout is the timeout for releasing a lease during shutdown.
	defaultReleaseTimeout = 5 * time.Second

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

// LeaseCallbacks defines callbacks for lease state changes.
type LeaseCallbacks struct {
	// OnAcquired is called when the lease is successfully acquired.
	OnAcquired func()

	// OnLost is called when the lease is lost (not voluntarily released).
	OnLost func()

	// OnRenewed is called when the lease is successfully renewed.
	OnRenewed func()

	// OnReleased is called when the lease is voluntarily released.
	OnReleased func()
}

// LeaseConfig holds configuration for a lease.
type LeaseConfig struct {
	// Key is the unique identifier for the lease in the KV store.
	Key string

	// TTL is the time-to-live for the lease.
	TTL time.Duration

	// RenewRatio is the ratio of TTL at which to renew (e.g., 0.5 means renew at 50% of TTL).
	RenewRatio float64

	// Value is the value to store with the lease (owner identifier).
	Value []byte

	// IsOwner checks if the given value belongs to this lease holder.
	IsOwner func(value []byte) bool

	// Callbacks for lease state changes.
	Callbacks LeaseCallbacks

	// Logger for lease operations.
	Logger *slog.Logger

	// Collector for metrics collection.
	Collector metrics.Collector
}

// Lease represents a distributed lease that can be acquired, renewed, and released.
type Lease struct {
	ops     *KVOps
	config  LeaseConfig
	metrics *leaseMetrics

	isHeld atomic.Bool
	// value holds the lease payload (owner identifier) as an atomically
	// swappable pointer. The camping goroutine reads it on every Renew
	// while UpdateValue may be called concurrently from a callback
	// goroutine (e.g. dlock.OnRenewed) — using atomic.Pointer keeps
	// reads and writes race-free without locking the hot Renew path.
	value atomic.Pointer[[]byte]

	// cancelMu guards cancel: RunCamping publishes it and StopCamping reads
	// it, and nothing requires those to happen on the same goroutine.
	cancelMu sync.Mutex
	cancel   context.CancelFunc

	// releaseGuard (a one-slot semaphore, so waiting honors a context)
	// serializes Release: stopping the camping loop makes it release the
	// lease too, and a caller must not return while that attempt — which may
	// fail and leave the lease held — is still in flight.
	releaseGuard chan struct{}

	logger *slog.Logger
}

func leaseLoggers(cfg LeaseConfig) (base *slog.Logger, scoped *slog.Logger) {
	base = cmp.Or(cfg.Logger, slog.Default())
	scoped = base.With(slog.String("key", cfg.Key))
	return base, scoped
}

// NewLease creates a new Lease instance.
func NewLease(kv jetstream.KeyValue, cfg LeaseConfig) *Lease {
	logger, scoped := leaseLoggers(cfg)

	l := &Lease{
		ops:     NewKVOps(kv, logger),
		config:  cfg,
		metrics: newLeaseMetrics(cfg.Collector),
		logger:  scoped,

		releaseGuard: make(chan struct{}, 1),
	}
	// Seed the race-safe value from the initial config. Take a defensive
	// copy so a caller who mutates the slice they passed in does not
	// race with the renew goroutine reading it.
	initial := append([]byte(nil), cfg.Value...)
	l.value.Store(&initial)
	return l
}

// currentValue returns the most-recently-set lease payload (initial
// LeaseConfig.Value, or whatever the latest UpdateValue installed).
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
	// Try to create the key - only succeeds if it doesn't exist
	_, err := l.ops.Create(ctx, l.config.Key, l.currentValue())
	if err == nil {
		l.isHeld.Store(true)
		l.metrics.leaseHeld.Set(1)
		l.record("acquire", nil)
		if l.config.Callbacks.OnAcquired != nil {
			l.config.Callbacks.OnAcquired()
		}
		return true, nil
	}

	// If key exists, check if we're the owner or if it's stale
	if errors.Is(err, jetstream.ErrKeyExists) {
		return l.tryTakeoverStale(ctx)
	}

	l.record("acquire", err)

	return false, err
}

// tryTakeoverStale attempts to take over a potentially stale lease.
func (l *Lease) tryTakeoverStale(ctx context.Context) (bool, error) {
	entry, err := l.ops.Get(ctx, l.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Key was deleted, try to acquire again
			return l.Acquire(ctx)
		}
		return false, err
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil {
		return false, nil
	}

	// Check if we're already the owner
	if l.config.IsOwner != nil && l.config.IsOwner(entry.Value()) {
		// We own it, just update to renew
		_, err := l.ops.Update(ctx, l.config.Key, l.currentValue(), entry.Revision())
		if err == nil {
			l.isHeld.Store(true)
			return true, nil
		}
		// Update failed, someone else got it
		return false, nil
	}

	// Someone else owns it
	return false, nil
}

// record counts one lease operation, classifying it by whether it failed.
func (l *Lease) record(op string, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	l.metrics.operations.WithLabels(metrics.Labels{"op": op, "result": result}).Inc()
}

// lost marks the lease as no longer held and reports it as a failed renewal.
// Every way of discovering the loss — the key gone, a tombstone, another owner,
// a revision that moved on — leaves the same state behind.
func (l *Lease) lost() (bool, error) {
	l.isHeld.Store(false)
	l.metrics.leaseHeld.Set(0)
	l.record("renew", ErrLeaseNotHeld)

	return false, ErrLeaseNotHeld
}

// Renew attempts to renew the lease.
// Returns true if successfully renewed, false if lease was lost.
func (l *Lease) Renew(ctx context.Context) (bool, error) {
	entry, err := l.ops.Get(ctx, l.config.Key)
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
	if l.config.IsOwner != nil && !l.config.IsOwner(entry.Value()) {
		return l.lost()
	}

	// Rewrite the key at the revision we just read, which resets its TTL. The
	// revision makes this a compare-and-set: a mismatch means the key moved on
	// without us, so the lease is gone rather than merely unwritable.
	if _, err = l.ops.Update(ctx, l.config.Key, l.currentValue(), entry.Revision()); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return l.lost()
		}
		l.record("renew", err)

		return false, err
	}

	l.record("renew", nil)

	if l.config.Callbacks.OnRenewed != nil {
		l.config.Callbacks.OnRenewed()
	}

	return true, nil
}

// Release voluntarily releases the lease and stops renewing it.
//
// It is idempotent: the deferred release in a caller's critical section, a
// provider shutting down, and the renewal loop's own teardown can all reach the
// same lease, and exactly one of them performs the delete.
// Concurrent calls are serialized, so none returns while another is still
// releasing; a release that fails leaves the lease held, and a later Release
// retries it.
func (l *Lease) Release(ctx context.Context) error {
	select {
	case l.releaseGuard <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-l.releaseGuard }()

	if !l.isHeld.CompareAndSwap(true, false) {
		return nil
	}

	// Stop renewing before touching the key, so the renewal loop cannot write
	// it back between the ownership check and the delete below.
	l.StopCamping()
	l.metrics.leaseHeld.Set(0)

	entry, err := l.ops.Get(ctx, l.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			l.released()
			return nil
		}
		return l.releaseFailed(err)
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil {
		l.released()
		return nil
	}

	// Someone else holds the key now — ours expired and was taken over.
	// Deleting it would be releasing a lock we no longer own.
	if l.config.IsOwner != nil && !l.config.IsOwner(entry.Value()) {
		l.logger.WarnContext(ctx, "lease was taken over before release; leaving the current holder's key alone")
		l.record("release", nil)

		return nil
	}

	// Revision-checked: between the Get above and this delete the key can
	// expire and be re-acquired by another holder, and an unconditional delete
	// would drop *their* lock. The revision makes the delete a compare-and-set.
	if err := l.ops.DeleteWithRevision(ctx, l.config.Key, entry.Revision()); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			l.released()
			return nil
		}
		return l.releaseFailed(err)
	}

	l.released()
	return nil
}

// releaseFailed records a release that did not reach the key and marks the
// lease held again, so a later Release retries it instead of returning as a
// no-op; renewal stays stopped, so an unretried lease lapses at its TTL.
func (l *Lease) releaseFailed(err error) error {
	l.record("release", err)
	l.isHeld.Store(true)
	l.metrics.leaseHeld.Set(1)

	return err
}

// released records a completed release and fires the callback.
func (l *Lease) released() {
	if l.config.Callbacks.OnReleased != nil {
		l.config.Callbacks.OnReleased()
	}
	l.record("release", nil)
}

// IsHeld returns true if the lease is currently held.
func (l *Lease) IsHeld() bool {
	return l.isHeld.Load()
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
	// its TTL, only after that, so the lease is valid at least until here + TTL.
	acquiredAt := time.Now()

	acquireCtx, cancelAcquire := corectx.ApplyTimeout(ctx, acquireTimeout)
	acquired, err := l.Acquire(acquireCtx)
	cancelAcquire()

	if err != nil {
		return false, err
	}

	if !acquired {
		return false, nil
	}

	campCtx, cancel := context.WithCancel(ctx)

	l.cancelMu.Lock()
	l.cancel = cancel
	l.cancelMu.Unlock()

	go func() {
		// Repo rule: every spawned goroutine ships with panics.Handle,
		// otherwise a panic in the renew loop (a callback that throws,
		// a NATS-driver bug, etc.) takes the whole process down.
		defer panics.Handle(campCtx)
		l.campingLoop(campCtx, acquiredAt)
	}()

	return true, nil
}

// StopCamping stops the renewal loop. It does not release the lease; the key is
// left for its TTL to reap. [Lease.Release] calls it.
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
	return time.Duration(float64(l.config.TTL) * renewSafetyMarginRatio)
}

// renewEvery is the camping loop's renewal cadence: [RenewInterval], clamped so
// a renewal always starts at least one safety margin before the renewal
// deadline. A ratio close to 1 would otherwise schedule the renewal at, or
// past, the point the lease may already have expired.
func (l *Lease) renewEvery() time.Duration {
	return min(RenewInterval(l.config.TTL, l.config.RenewRatio), l.config.TTL-2*l.safetyMargin())
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

	backoff := coreretry.Exponential(coreretry.ExponentialConfig{
		BaseDelay: min(retryBaseDelay, every),
		MaxDelay:  every,
		Jitter:    DefaultJitter,
	})

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
func (l *Lease) campingLoop(ctx context.Context, acquiredAt time.Time) {
	every := l.renewEvery()
	writtenAt := acquiredAt

	timer := time.NewTimer(time.Until(writtenAt.Add(every)))
	defer timer.Stop()

	l.logger.DebugContext(ctx, "camping loop started", slog.Duration("renew_interval", every))
	defer l.logger.DebugContext(ctx, "camping loop stopped")

	for {
		select {
		case <-timer.C:
			renewedAt, err := l.renewWithRetry(ctx, writtenAt.Add(l.config.TTL), every)
			if err == nil {
				writtenAt = renewedAt
				timer.Reset(time.Until(writtenAt.Add(every)))
				l.logger.DebugContext(ctx, "lease renewed")

				continue
			}

			// Canceled mid-renewal: the ctx.Done branch releases the lease.
			if ctx.Err() != nil {
				continue
			}

			// Renew marks a definitive loss itself; a lease that ran out of time
			// is just as gone.
			if !errors.Is(err, ErrLeaseNotHeld) {
				l.isHeld.Store(false)
				l.metrics.leaseHeld.Set(0)
			}
			l.logger.WarnContext(ctx, "lease lost during renewal", slog.Any("error", err))
			if l.config.Callbacks.OnLost != nil {
				l.config.Callbacks.OnLost()
			}

			return

		case <-ctx.Done():
			l.logger.DebugContext(ctx, "context canceled, releasing lease")
			releaseCtx, cancel := context.WithTimeout(context.Background(), defaultReleaseTimeout)
			if err := l.Release(releaseCtx); err != nil { //nolint:contextcheck // original context canceled
				//nolint:contextcheck // using new context for cleanup after original context canceled
				l.logger.ErrorContext(releaseCtx, "failed to release lease", slog.Any("error", err))
			}
			cancel()
			return
		}
	}
}
