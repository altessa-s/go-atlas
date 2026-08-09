// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
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

func (lt *lockTracker) Delete(key string) {
	lt.mu.Lock()
	delete(lt.locks, key)
	lt.mu.Unlock()
}

func (lt *lockTracker) Range(fn func(key string, lock *lock) bool) {
	lt.mu.RLock()
	// Copy map to avoid holding lock during iteration
	locks := make(map[string]*lock, len(lt.locks))
	for k, v := range lt.locks {
		locks[k] = v
	}
	lt.mu.RUnlock()

	for k, v := range locks {
		if !fn(k, v) {
			break
		}
	}
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
}

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
	l.kv, err = kvHelper.GetOrCreateBucket(ctx, natskvlease.BucketConfig{
		Bucket:      l.opts.bucket,
		TTL:         l.opts.ttl,
		Storage:     jetstream.MemoryStorage,
		Compression: true,
	})
	if err != nil {
		return nil, err
	}

	l.kvOps = natskvlease.NewKVOps(l.kv, l.opts.logger)

	return l, nil
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
	// Check if provider is closed
	if l.closed.Load() {
		return nil, errors.New("provider is closed")
	}

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

	// Track active lock
	l.activeLocks.Store(key, lk)

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
	entry, err := l.kvOps.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, errs.ErrLockNotHeld
		}
		return nil, err
	}

	// Check if entry or its value is nil (defensive check for tombstones)
	if entry == nil || entry.Value() == nil {
		return nil, errs.ErrLockNotHeld
	}

	var metadata lockMetadata
	if err := json.Unmarshal(entry.Value(), &metadata); err != nil {
		return nil, err
	}

	now := time.Now()
	isStale := now.Sub(metadata.LastRenewed) > time.Duration(metadata.TTL)

	return &providers.LockInfo{
		Key:          key,
		Owner:        metadata.OwnerId,
		AcquiredAt:   metadata.AcquiredAt,
		LastRenewed:  metadata.LastRenewed,
		TTL:          time.Duration(metadata.TTL),
		IsStale:      isStale,
		FencingToken: entry.Revision(),
	}, nil
}

// Close closes the provider and releases all resources.
// It releases all active locks before closing.
func (l *Locker) Close(ctx context.Context) error {
	// Mark as closed to prevent new locks
	if !l.closed.CompareAndSwap(false, true) {
		return errors.New("provider already closed")
	}

	if l.opts.logger != nil {
		l.opts.logger.InfoContext(ctx, "closing NATS dlock provider")
	}

	// Release all active locks
	var releaseErrors []error
	l.activeLocks.Range(func(key string, lock *lock) bool {
		releaseCtx, cancel := corectx.ApplyTimeout(ctx, DefaultOperationsTimeout)
		if err := lock.Release(releaseCtx); err != nil {
			releaseErrors = append(releaseErrors, coreerrs.Wrapf(err, "failed to release lock %s", key))
		}
		cancel()
		l.activeLocks.Delete(key)
		return true
	})

	// Log any release errors
	if len(releaseErrors) > 0 {
		for _, err := range releaseErrors {
			if l.opts.logger != nil {
				l.opts.logger.ErrorContext(ctx, "error during provider close", slog.Any("error", err))
			}
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
		return errors.New("provider is closed")
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

// Release releases the lock with context support.
func (t *trackedLock) Release(ctx context.Context) error {
	err := t.lock.Release(ctx)
	t.tracker.Delete(t.key)
	return err
}
