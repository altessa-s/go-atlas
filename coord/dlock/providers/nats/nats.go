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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrBucketTTLMismatch is returned by New when the bucket already exists with a
// key TTL other than the configured one and [WithMigrateBucketTTL] is not set.
// The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by New when the bucket already exists
// with another storage type. The bucket is left untouched:
// the server cannot convert a bucket's storage type.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// ErrKeyTooLong is returned, before any network I/O, for a lock key whose
// escaped form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

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

// Snapshot copies the tracked locks so shutdown can release them outside the mutex.
func (lt *lockTracker) Snapshot() []*lock {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	locks := make([]*lock, 0, len(lt.locks))
	for _, lk := range lt.locks {
		locks = append(locks, lk)
	}
	return locks
}

// Locker implements the provider.Locker interface and the providers.Provider interface using NATS Key Value.
// It provides distributed locking capabilities using NATS JetStream's KV.
type Locker struct {
	client   *nats.Conn
	js       jetstream.JetStream
	opts     *options
	kv       jetstream.KeyValue
	kvOps    *natskvlease.KVOps
	observer natskvlease.LeaseObserver

	// Track active locks for cleanup
	activeLocks *lockTracker
	closed      atomic.Bool

	// mu orders Lock's closed check and registration against Close, and
	// inflight counts the acquisitions past that check, which Close waits
	// for: an acquisition completing while Close runs is either registered
	// before Close collects the locks or released by Lock itself.
	mu       sync.Mutex
	inflight sync.WaitGroup
	drained  chan struct{}
}

// errClosed reports an operation on a closed provider.
var errClosed = errors.New("provider is closed")

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
)

// config holds the internal configuration for a lock.
type config struct {
	Key string
	TTL time.Duration
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

	if cfg.meter != nil {
		l.observer = newLeaseMetrics(cfg.meter)
	}
	l.kvOps = natskvlease.NewKVOps(l.kv, l.opts.logger)

	return l, nil
}

// bucketConfig is the bucket New creates or reconciles.
func bucketConfig(o *options) natskvlease.BucketSpec {
	return natskvlease.BucketSpec{
		Bucket:      o.bucket,
		TTL:         o.ttl,
		Storage:     o.storage,
		Compression: true,
		MigrateTTL:  o.migrateBucketTTL,
	}
}

// Lock makes a single attempt to acquire the lock for key.
//
// It does not wait for a current holder: when the key is taken the attempt
// returns [providers.ErrLockNotHeld] straight away. Callers that need to be
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

	lk := newLock(&config{Key: key, TTL: l.opts.ttl}, l.kvOps, l.opts, l.observer)
	lk.onTerminal = func() { l.activeLocks.DeleteIf(key, lk) }
	ok, err := lk.run(ctx, l.opts.acquireTimeout)
	if errors.Is(err, natskvlease.ErrLeaseNotHeld) {
		return nil, providers.ErrLockNotHeld
	}
	if err != nil {
		if l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "failed to acquire lock", slog.Any("error", err), slog.String("key", key))
		}
		return nil, err
	}
	if !ok {
		return nil, providers.ErrLockNotHeld
	}

	l.mu.Lock()
	closed := l.closed.Load()
	l.activeLocks.Store(key, lk)
	l.mu.Unlock()
	if lk.terminal.Load() { // ended before it was tracked
		l.activeLocks.DeleteIf(key, lk)
	}
	if closed {
		releaseCtx, cancel := corectx.ApplyTimeout(context.WithoutCancel(ctx), DefaultOperationsTimeout)
		defer cancel()
		if err := lk.Release(releaseCtx); err != nil && l.opts.logger != nil {
			l.opts.logger.ErrorContext(ctx, "failed to release a lock acquired while closing",
				slog.Any("error", err), slog.String("key", key))
		}
		return nil, errClosed
	}

	return lk, nil
}

// GetLockInfo retrieves metadata about the lock identified by key.
// Returns [providers.ErrLockNotHeld] if the key has no active lock.
func (l *Locker) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return readLockInfo(ctx, l.kvOps, key)
}

// Close rejects new acquisitions, invalidates active lock handles, stops
// renewal, and releases the leases. A later Close can retry pending releases.
func (l *Locker) Close(ctx context.Context) error {
	l.mu.Lock()
	l.closed.Store(true)
	held := l.activeLocks.Snapshot()
	for _, lk := range held {
		lk.isLocked.Store(false)
		lk.lease.StopCamping()
	}
	// Wait for acquisitions that had already passed the closed check.
	if l.drained == nil {
		l.drained = make(chan struct{})
		go func() {
			defer panics.Handle(ctx)
			l.inflight.Wait()
			close(l.drained)
		}()
	}
	drained := l.drained
	l.mu.Unlock()
	select {
	case <-drained:
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for in-flight lock acquisitions")
	}

	if l.opts.logger != nil {
		l.opts.logger.InfoContext(ctx, "closing NATS dlock provider")
	}

	var releaseErrs []error
	for _, lk := range l.activeLocks.Snapshot() {
		releaseCtx, cancel := corectx.WithMaxTimeout(ctx, DefaultOperationsTimeout)
		err := lk.Release(releaseCtx)
		cancel()

		if err != nil {
			releaseErrs = append(releaseErrs, coreerrs.Wrapf(err, "release lock %q", lk.config.Key))
		}
	}
	return errors.Join(releaseErrs...)
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
