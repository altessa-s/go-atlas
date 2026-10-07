// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrBucketTTLMismatch is returned by New when the bucket already exists with a
// key TTL other than the configured one and [WithMigrateBucketTTL] is not set.
// The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by New when the bucket already exists
// with another storage type and [WithStrictBucketStorage] is set. The bucket is
// left untouched.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// lockTracker efficiently tracks active locks for cleanup
type lockTracker struct {
	mu    sync.RWMutex
	locks map[string]*lock
}

func newLockTracker() *lockTracker {
	return &lockTracker{
		locks: make(map[string]*lock, 32), // Pre-allocate for better performance
	}
}

func (lt *lockTracker) Store(key string, lock *lock) {
	lt.mu.Lock()
	lt.locks[key] = lock
	lt.mu.Unlock()
}

// DeleteIf untracks key only while it still maps to lk. A release of an
// older lock for the same key must not untrack the newer one, or Close
// would miss it.
func (lt *lockTracker) DeleteIf(key string, lk *lock) {
	lt.mu.Lock()
	if lt.locks[key] == lk {
		delete(lt.locks, key)
	}
	lt.mu.Unlock()
}

// Drain removes every tracked lock and returns them.
//
// Taking the whole set in one swap keeps the shutdown release loop off the
// mutex while it does network I/O, and leaves nothing behind for a second
// caller to release twice.
func (lt *lockTracker) Drain() map[string]*lock {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	locks := lt.locks
	lt.locks = make(map[string]*lock)

	return locks
}

// Locker implements the provider.Locker interface and the providers.Provider interface using NATS Key Value.
// It provides distributed locking capabilities using NATS JetStream's KV.
type Locker struct {
	client *nats.Conn
	js     jetstream.JetStream
	opts   *options
	kv     jetstream.KeyValue
	kvOps  *natskvlease.KVOps

	// Track active locks for cleanup
	activeLocks *lockTracker
	closed      atomic.Bool

	// mu orders Lock's closed check and registration against Close, and
	// inflight counts the acquisitions past that check, which Close waits
	// for: an acquisition completing while Close runs is either registered
	// before Close drains the locks or released by Lock itself.
	mu       sync.Mutex
	inflight sync.WaitGroup
}

// errClosed reports an operation on a closed provider.
var errClosed = errors.New("provider is closed")

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
)

// config holds the internal configuration for a lock.
type config struct {
	Key   string
	TTL   time.Duration
	Value string
}

// New creates a new NATS KV-based distributed lock.
// It requires a NATS client connection and accepts optional configuration
// through functional options.
func New(ctx context.Context, client *nats.Conn, opts ...Option) (*Locker, error) {
	cfg := newOptions(opts...)

	l := &Locker{
		client:      client,
		opts:        cfg,
		activeLocks: newLockTracker(),
	}

	var err error
	l.js, err = jetstream.New(client)
	if err != nil {
		if l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "failed to create JetStream context", slog.Any("error", err))
		}
		return nil, err
	}

	// Validate JetStream is enabled
	if jsErr := natskvlease.ValidateJetStreamEnabled(ctx, l.js, l.opts.logger); jsErr != nil {
		return nil, jsErr
	}

	// The bucket's key TTL is what reaps a lock whose holder died without
	// releasing it, so it has to be the lock TTL. Pinning it to a constant
	// while the lock TTL came from options meant a longer WithTTL produced a
	// lock the server aged out early — the renewal was scheduled off the
	// configured TTL and the key was gone before it ever fired.
	kvHelper := natskvlease.NewKVHelper(l.js, l.opts.logger)
	l.kv, err = kvHelper.GetOrCreateBucket(ctx, bucketConfig(l.opts))
	if err != nil {
		return nil, err
	}

	l.kvOps = natskvlease.NewKVOps(l.kv, l.opts.logger)

	return l, nil
}

// bucketConfig is the bucket New creates and MigrateBucketStorage migrates to.
func bucketConfig(o *options) natskvlease.BucketConfig {
	return natskvlease.BucketConfig{
		Bucket:        o.bucket,
		TTL:           o.ttl,
		Storage:       o.storage,
		Compression:   true,
		MigrateTTL:    o.migrateBucketTTL,
		StrictStorage: o.strictBucketStorage,
	}
}

// Lock makes a single attempt to acquire the lock for key.
//
// It does not wait for a current holder: when the key is taken the attempt
// returns [errs.ErrLockNotHeld] straight away. Callers that need to be
// serialized rather than rejected must retry.
//
// ctx scopes the lock. Cancel it and the lease stops being renewed and is
// released, so a lock must not be handed a context that ends before the work
// it guards. The acquisition attempt itself is bounded separately, by
// [WithAcquireTimeout]. The returned [providers.Lock] should still be released
// explicitly when the work is done.
func (l *Locker) Lock(ctx context.Context, key string) (providers.Lock, error) {
	l.mu.Lock()
	if l.closed.Load() {
		l.mu.Unlock()
		return nil, errClosed
	}
	l.inflight.Add(1)
	l.mu.Unlock()
	defer l.inflight.Done()

	lk := newLock(&config{Key: key, TTL: l.opts.ttl, Value: uuid.NewString()}, l.kvOps, l.opts.logger, l.opts.renewRatio)
	ok, err := lk.run(ctx, l.opts.acquireTimeout)
	if err != nil {
		if l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "failed to acquire lock", slog.Any("error", err), slog.String("key", key))
		}
		return nil, err
	} else if !ok {
		return nil, errs.ErrLockNotHeld
	}

	l.mu.Lock()
	if l.closed.Load() {
		l.mu.Unlock()
		releaseCtx, cancel := corectx.ApplyTimeout(context.WithoutCancel(ctx), DefaultOperationsTimeout)
		defer cancel()
		if err := lk.Release(releaseCtx); err != nil && l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "failed to release a lock acquired while closing",
				slog.Any("error", err), slog.String("key", key))
		}
		return nil, errClosed
	}
	l.activeLocks.Store(key, lk)
	l.mu.Unlock()

	// Wrap the lock to remove from tracking on release
	return &trackedLock{
		lock:    lk,
		key:     key,
		tracker: l.activeLocks,
	}, nil
}

// GetLockInfo retrieves metadata about the lock identified by key.
// Returns [errs.ErrLockNotHeld] if the key has no active lock.
func (l *Locker) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return readLockInfo(ctx, l.kvOps, key)
}

// Close closes the provider and releases all resources.
// It releases all active locks before closing.
func (l *Locker) Close(ctx context.Context) error {
	// Mark as closed to prevent new locks
	l.mu.Lock()
	if !l.closed.CompareAndSwap(false, true) {
		l.mu.Unlock()
		return errors.New("provider already closed")
	}
	l.mu.Unlock()

	// Acquisitions already past the closed check register before the drain
	// below or release themselves; wait until they have done either.
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

	if l.opts.logger != nil {
		l.opts.logger.InfoContext(ctx, "closing NATS dlock provider")
	}

	// Release all active locks. Each gets its own budget so one unreachable
	// key cannot consume the whole shutdown.
	for key, lk := range l.activeLocks.Drain() {
		releaseCtx, cancel := corectx.ApplyTimeout(ctx, DefaultOperationsTimeout)
		err := lk.Release(releaseCtx)
		cancel()

		if err != nil && l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "error during provider close",
				slog.Any("error", coreerrs.Wrapf(err, "failed to release lock %s", key)))
		}
	}

	// Note: We don't close the NATS connection as it's managed externally
	// The caller who provided the connection is responsible for closing it

	if l.opts.logger != nil {
		l.opts.logger.InfoContext(ctx, "nats dlock provider closed successfully")
	}
	return nil
}

// Probe implements [providers.Prober]. It returns nil when the NATS
// connection is established and the KV bucket is reachable. Used by
// [dlock.DLock.CheckHealth] for readiness probes.
func (l *Locker) Probe(ctx context.Context) error {
	if l.closed.Load() {
		return errClosed
	}
	if l.client == nil || l.client.Status() != nats.CONNECTED {
		return errors.New("nats client not connected")
	}
	if l.kv == nil {
		return errors.New("kv bucket not initialized")
	}
	if _, err := l.kv.Status(ctx); err != nil {
		return coreerrs.Wrap(err, "kv bucket unreachable")
	}
	return nil
}

// trackedLock wraps a lock to track it in the provider
type trackedLock struct {
	lock    *lock
	key     string
	tracker *lockTracker
}

// GetLockInfo returns information about the current state of the lock.
func (t *trackedLock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	return t.lock.GetLockInfo(ctx)
}

// Release releases the lock with context support. A failed release keeps the
// lock tracked, so it can be retried — by Release or by Close.
func (t *trackedLock) Release(ctx context.Context) error {
	if err := t.lock.Release(ctx); err != nil {
		return err
	}
	t.tracker.DeleteIf(t.key, t.lock)
	return nil
}
