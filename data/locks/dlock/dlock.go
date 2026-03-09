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

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/noop"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	natsio "github.com/nats-io/nats.go"
)

// DLock provides distributed locking with configurable providers.
// It is safe for concurrent use.
type DLock struct {
	provider           providers.Provider
	logger             *slog.Logger
	lockAcquireTimeout time.Duration
}

// New creates a new DLock with the specified provider.
//
// Example:
//
//	dl := dlock.New(natsProvider)
func New(provider providers.Provider, opts ...Option) *DLock {
	cfg := newOptions(opts...)

	return &DLock{
		provider:           provider,
		logger:             cfg.logger,
		lockAcquireTimeout: cfg.lockAcquireTimeout,
	}
}

// NewWithNats creates a DLock with a NATS JetStream provider.
//
// Example:
//
//	dl, err := dlock.NewWithNats(ctx, conn, "locks")
func NewWithNats(ctx context.Context, conn *natsio.Conn, bucket string, opts ...Option) (*DLock, error) {
	prov, err := nats.New(ctx, conn,
		nats.WithBucket(bucket),
	)

	if err != nil {
		return nil, coreerrs.Provider("nats distributed lock", err)
	}

	return New(prov, opts...), nil
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
// Example:
//
//	err := dl.Synchronize(ctx, "resource", func(ctx context.Context) error {
//		return doWork(ctx)
//	})
func (l *DLock) Synchronize(ctx context.Context, key string, fn func(ctx context.Context) error) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	// Apply timeout protection if configured to prevent indefinite blocking
	lockCtx, lockCancel := corectx.ApplyTimeout(ctx, l.lockAcquireTimeout)
	defer lockCancel()

	// Acquire lock with timeout protection to prevent deadlock scenarios
	lk, err := l.provider.Lock(lockCtx, key)
	if err != nil {
		if coreerrs.IsContextDeadlineExceeded(err) {
			return coreerrs.Wrapf(err, "failed to acquire lock within timeout %v", l.lockAcquireTimeout)
		}
		return err
	}

	// Track if lock has been released to prevent double-release
	var lockReleased bool
	var lockMutex sync.Mutex

	// Safe release function that ensures lock is only released once
	safeRelease := func() { //nolint:contextcheck
		lockMutex.Lock()
		defer lockMutex.Unlock()
		if !lockReleased {
			_ = lk.Release(context.Background()) //nolint:errcheck
			lockReleased = true
		}
	}

	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, r any) {
		safeRelease()
		l.logger.ErrorContext(ctx, "panic in synchronize", slog.Any("panic", r))
	})

	// Execute the provided function while holding the lock.
	// Use the original context to maintain proper cancellation semantics
	err = fn(ctx)
	safeRelease() // Ensure the lock is released after execution
	return err
}

// GetLockInfo returns the current state of a lock.
func (l *DLock) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}
	return l.provider.GetLockInfo(ctx, key)
}

// Lock acquires a distributed lock. Blocks until acquired or context canceled.
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
