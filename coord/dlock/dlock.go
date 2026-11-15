// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/noop"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

// ErrLockLost is the cancellation cause of the context [DLock.Synchronize]
// passes to its function, and is matched by the error Synchronize returns,
// when the lock's lease was lost while the function ran.
var ErrLockLost = errors.New("dlock: lock lost")

// DLock provides distributed locking with configurable providers.
// It is safe for concurrent use.
type DLock struct {
	provider       providers.Provider
	logger         *slog.Logger
	metrics        *dlockMetrics
	releaseTimeout time.Duration
}

// New creates a new DLock with the specified provider.
//
// Example:
//
//	dl := dlock.New(natsProvider)
func New(provider providers.Provider, opts ...Option) *DLock {
	cfg := newOptions(opts...)

	l := &DLock{
		provider:       provider,
		logger:         cfg.logger,
		metrics:        newDlockMetrics(cfg.meter),
		releaseTimeout: cfg.releaseTimeout,
	}

	if cfg.healthCoordinator != nil {
		cfg.healthCoordinator.RegisterService(cfg.healthServiceName, l)
	}

	return l
}

// NewWithNoop creates a DLock with a no-op provider for testing.
//
// Example:
//
//	dl := dlock.NewWithNoop()
func NewWithNoop(opts ...Option) *DLock {
	return New(noop.New(), opts...)
}

// Synchronize acquires a lock, executes fn, and releases the lock.
// Returns error if lock acquisition fails.
//
// When the lock's lease is lost while fn runs — it expired, was taken over or
// could not be renewed — and the provider's lock reports it
// ([providers.LostNotifier]; the mongo and sqldb providers do), the context
// passed to fn is canceled with cause [ErrLockLost] and Synchronize returns
// an error matching [ErrLockLost] joined with fn's own. fn must stop its work
// when its context ends: the key may already have another holder.
//
// Example:
//
//	err := dl.Synchronize(ctx, "resource", func(ctx context.Context) error {
//		return doWork(ctx)
//	})
func (l *DLock) Synchronize(ctx context.Context, key string, fn func(ctx context.Context) error) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	// The caller's context is handed to the provider unchanged: it scopes the
	// lock, and wrapping it in an acquisition deadline here would release the
	// lock the moment that deadline passed — while fn was still running. The
	// provider bounds its own acquisition attempt.
	stop := l.metrics.acquireDuration.Start()
	lk, err := l.provider.Lock(ctx, key)
	stop()
	if err != nil {
		l.metrics.locksFailed.Inc()
		return err
	}
	l.metrics.locksAcquired.Inc()

	// Track if lock has been released to prevent double-release
	var lockReleased bool
	var lockMutex sync.Mutex

	// Safe release function that ensures lock is only released once.
	// Increments locks_released_total only on successful release so the
	// metric stays a true positive signal for leak detection (paired
	// with locks_acquired_total). The release outlives ctx, bounded by the
	// release timeout; a failed one is logged and not recorded as released.
	safeRelease := func() {
		lockMutex.Lock()
		defer lockMutex.Unlock()
		if lockReleased {
			return
		}
		releaseCtx, cancel := corecontext.ApplyTimeout(context.WithoutCancel(ctx), l.releaseTimeout)
		defer cancel()
		if releaseErr := lk.Release(releaseCtx); releaseErr != nil {
			l.logger.ErrorContext(releaseCtx, "failed to release lock", slog.Any("error", releaseErr), slog.String("key", key))
			return
		}
		l.metrics.locksReleased.Inc()
		lockReleased = true
	}

	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, r any) {
		safeRelease()
		l.logger.ErrorContext(ctx, "panic in synchronize", slog.Any("panic", r))
	})

	// fn runs under a context that also ends, with cause ErrLockLost, when a
	// lock that can tell (providers.LostNotifier) loses its lease.
	fnCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var lost <-chan struct{}
	if notifier, ok := lk.(providers.LostNotifier); ok {
		lost = notifier.Lost()
		finished := make(chan struct{})
		defer close(finished)
		go func() {
			select {
			case <-lost:
				cancel(ErrLockLost)
			case <-finished:
			}
		}()
	}

	// Execute the provided function while holding the lock.
	err = fn(fnCtx)
	// Read the loss directly: the goroutine above may not have run yet, and a
	// nil channel never reports one.
	wasLost := false
	select {
	case <-lost:
		wasLost = true
	default:
	}
	safeRelease() // Ensure the lock is released after execution
	l.metrics.synchronizations.Inc()
	if wasLost {
		return errors.Join(ErrLockLost, err)
	}
	return err
}

// GetLockInfo returns the current state of a lock.
func (l *DLock) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}
	return l.provider.GetLockInfo(ctx, key)
}

// Lock makes a single attempt to acquire a distributed lock.
//
// It does not wait for a current holder: when the key is taken, the provider
// returns its "not held" error straight away. Callers that need to be
// serialized rather than rejected must retry.
//
// ctx scopes the lock, not just the call. Canceling it stops the lease being
// renewed and releases the lock, so do not pass a context that ends before the
// work the lock guards.
//
// Example:
//
//	lock, err := dl.Lock(ctx, "resource")
//	defer lock.Release(ctx)
func (l *DLock) Lock(ctx context.Context, key string) (providers.Lock, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	// Acquire the lock using the provider.
	return l.provider.Lock(ctx, key)
}

// Close releases all resources held by the DLock.
func (l *DLock) Close(ctx context.Context) error {
	l.logger.DebugContext(ctx, "closing DLock instance")
	if nilcheck.IsNotNil(l.provider) {
		return l.provider.Close(ctx)
	}
	return nil
}

var _ Locker = (*DLock)(nil)
