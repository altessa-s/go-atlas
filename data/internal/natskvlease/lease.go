// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Common errors for lease operations.
var (
	ErrLeaseNotHeld = errors.New("lease not held")
	ErrLeaseExists  = errors.New("lease already exists")
)

// defaultReleaseTimeout is the timeout for releasing a lease during shutdown.
const defaultReleaseTimeout = 5 * time.Second

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
	value  atomic.Pointer[[]byte]
	cancel context.CancelFunc
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
		l.metrics.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
		if l.config.Callbacks.OnAcquired != nil {
			l.config.Callbacks.OnAcquired()
		}
		return true, nil
	}

	// If key exists, check if we're the owner or if it's stale
	if errors.Is(err, jetstream.ErrKeyExists) {
		return l.tryTakeoverStale(ctx)
	}

	l.metrics.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
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

// Renew attempts to renew the lease.
// Returns true if successfully renewed, false if lease was lost.
func (l *Lease) Renew(ctx context.Context) (bool, error) {
	entry, err := l.ops.Get(ctx, l.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			l.isHeld.Store(false)
			l.metrics.leaseHeld.Set(0)
			l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
			return false, ErrLeaseNotHeld
		}
		l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, err
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil {
		l.isHeld.Store(false)
		l.metrics.leaseHeld.Set(0)
		l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, ErrLeaseNotHeld
	}

	// Check ownership
	if l.config.IsOwner != nil && !l.config.IsOwner(entry.Value()) {
		l.isHeld.Store(false)
		l.metrics.leaseHeld.Set(0)
		l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, ErrLeaseNotHeld
	}

	// Update with new value (potentially updated timestamp)
	_, err = l.ops.Update(ctx, l.config.Key, l.currentValue(), entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Revision mismatch - someone else modified
			l.isHeld.Store(false)
			l.metrics.leaseHeld.Set(0)
			l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
			return false, ErrLeaseNotHeld
		}
		l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, err
	}

	l.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "success"}).Inc()

	if l.config.Callbacks.OnRenewed != nil {
		l.config.Callbacks.OnRenewed()
	}

	return true, nil
}

// Release voluntarily releases the lease.
func (l *Lease) Release(ctx context.Context) error {
	if !l.isHeld.Load() {
		return nil
	}

	entry, err := l.ops.Get(ctx, l.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			l.isHeld.Store(false)
			return nil
		}
		return err
	}

	// Check if entry is valid
	if entry == nil || entry.Value() == nil {
		l.isHeld.Store(false)
		return nil
	}

	// Only delete if we're the owner
	if l.config.IsOwner != nil && l.config.IsOwner(entry.Value()) {
		if err := l.ops.Delete(ctx, l.config.Key); err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				l.metrics.operations.WithLabels(metrics.Labels{"op": "release", "result": "failure"}).Inc()
				return err
			}
		}
		if l.config.Callbacks.OnReleased != nil {
			l.config.Callbacks.OnReleased()
		}
	}

	l.isHeld.Store(false)
	l.metrics.leaseHeld.Set(0)
	l.metrics.operations.WithLabels(metrics.Labels{"op": "release", "result": "success"}).Inc()
	return nil
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

// RunCamping starts a camping loop that maintains the lease.
// It acquires the lease and periodically renews it until the context is canceled.
// Returns true if lease was acquired, false otherwise.
func (l *Lease) RunCamping(ctx context.Context) (bool, error) {
	ctx, l.cancel = context.WithCancel(ctx)

	acquired, err := l.Acquire(ctx)
	if err != nil {
		l.cancel() // Clean up derived context on error
		return false, err
	}

	if !acquired {
		l.cancel() // Clean up derived context when not acquired
		return false, nil
	}

	go func() {
		// Repo rule: every spawned goroutine ships with panics.Handle,
		// otherwise a panic in the renew loop (a callback that throws,
		// a NATS-driver bug, etc.) takes the whole process down.
		defer panics.Handle(ctx)
		l.campingLoop(ctx)
	}()
	return true, nil
}

// StopCamping stops the camping loop.
func (l *Lease) StopCamping() {
	if l.cancel != nil {
		l.cancel()
	}
}

// campingLoop maintains the lease by periodically renewing it.
func (l *Lease) campingLoop(ctx context.Context) {
	renewInterval := time.Duration(float64(l.config.TTL) * l.config.RenewRatio)
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	l.logger.DebugContext(ctx, "camping loop started", slog.Duration("renew_interval", renewInterval))
	defer l.logger.DebugContext(ctx, "camping loop stopped")

	for {
		select {
		case <-ticker.C:
			renewed, err := l.Renew(ctx)
			if err != nil {
				if coreerrs.IsContextCanceled(err) {
					return
				}
				l.logger.ErrorContext(ctx, "failed to renew lease", slog.Any("error", err))
				if l.config.Callbacks.OnLost != nil {
					l.config.Callbacks.OnLost()
				}
				return
			}
			if !renewed {
				l.logger.WarnContext(ctx, "lease lost during renewal")
				if l.config.Callbacks.OnLost != nil {
					l.config.Callbacks.OnLost()
				}
				return
			}
			l.logger.DebugContext(ctx, "lease renewed")

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
