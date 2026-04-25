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

	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Common errors for lease operations.
var (
	ErrLeaseNotHeld    = errors.New("lease not held")
	ErrLeaseExists     = errors.New("lease already exists")
	ErrProviderClosed  = errors.New("provider is closed")
	ErrProviderStopped = errors.New("provider is stopped")
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

	return &Lease{
		ops:     NewKVOps(kv, logger),
		config:  cfg,
		metrics: newLeaseMetrics(cfg.Collector),
		logger:  scoped,
	}
}

// Acquire attempts to acquire the lease.
// Returns true if successfully acquired, false if already held by another owner.
func (l *Lease) Acquire(ctx context.Context) (bool, error) {
	// Try to create the key - only succeeds if it doesn't exist
	_, err := l.ops.Create(ctx, l.config.Key, l.config.Value)
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
		_, err := l.ops.Update(ctx, l.config.Key, l.config.Value, entry.Revision())
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
	_, err = l.ops.Update(ctx, l.config.Key, l.config.Value, entry.Revision())
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
func (l *Lease) UpdateValue(val []byte) {
	l.config.Value = val
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

	go l.campingLoop(ctx)
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

// LeaseManager manages a lease with optional key watching for faster acquisition.
type LeaseManager struct {
	ops     *KVOps
	config  LeaseConfig
	metrics *leaseMetrics

	isHeld  atomic.Bool
	cancel  context.CancelFunc
	logger  *slog.Logger
	stopped atomic.Bool
}

// NewLeaseManager creates a new LeaseManager instance.
func NewLeaseManager(kv jetstream.KeyValue, cfg LeaseConfig) *LeaseManager {
	logger, scoped := leaseLoggers(cfg)

	return &LeaseManager{
		ops:     NewKVOps(kv, logger),
		config:  cfg,
		metrics: newLeaseMetrics(cfg.Collector),
		logger:  scoped,
	}
}

// Start begins the lease management loop.
// It continuously attempts to acquire and maintain the lease.
func (m *LeaseManager) Start(ctx context.Context) error {
	if m.stopped.Load() {
		return ErrProviderStopped
	}

	ctx, m.cancel = context.WithCancel(ctx)
	go m.managementLoop(ctx)
	return nil
}

// Stop stops the lease management loop and releases the lease.
func (m *LeaseManager) Stop(ctx context.Context) error {
	if !m.stopped.CompareAndSwap(false, true) {
		return ErrProviderStopped
	}

	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// IsHeld returns true if the lease is currently held.
func (m *LeaseManager) IsHeld() bool {
	return m.isHeld.Load()
}

// managementLoop is the main loop that manages the lease lifecycle.
func (m *LeaseManager) managementLoop(ctx context.Context) {
	renewInterval := time.Duration(float64(m.config.TTL) * m.config.RenewRatio)
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	// Try to set up a watcher for faster acquisition
	watcher, watchErr := m.ops.Watch(ctx, m.config.Key)
	if watchErr != nil {
		m.logger.WarnContext(ctx, "failed to create key watcher, falling back to polling",
			slog.Any("error", watchErr))
	} else {
		defer watcher.Stop() //nolint:errcheck
	}

	m.logger.DebugContext(ctx, "lease management loop started",
		slog.Duration("renew_interval", renewInterval))
	defer m.logger.DebugContext(ctx, "lease management loop stopped")

	// Initial acquisition attempt
	m.tryAcquire(ctx)

	for {
		select {
		case <-ticker.C:
			if m.isHeld.Load() {
				m.tryRenew(ctx)
			} else {
				m.tryAcquire(ctx)
			}

		case update := <-m.watcherUpdates(watcher):
			if update == nil {
				continue
			}
			// Key was deleted or purged - try to acquire
			if update.Operation() == jetstream.KeyValueDelete ||
				update.Operation() == jetstream.KeyValuePurge {
				m.logger.DebugContext(ctx, "key deleted/purged, attempting acquisition")
				m.tryAcquire(ctx)
			}

		case <-ctx.Done():
			m.logger.DebugContext(ctx, "context canceled, releasing lease")
			releaseCtx, cancel := context.WithTimeout(context.Background(), defaultReleaseTimeout)
			m.release(releaseCtx) //nolint:contextcheck
			cancel()
			return
		}
	}
}

// watcherUpdates returns the updates channel from the watcher, or a nil channel if no watcher.
func (m *LeaseManager) watcherUpdates(watcher jetstream.KeyWatcher) <-chan jetstream.KeyValueEntry {
	if watcher == nil {
		return nil
	}
	return watcher.Updates()
}

// tryAcquire attempts to acquire the lease.
func (m *LeaseManager) tryAcquire(ctx context.Context) {
	acquired, err := m.acquire(ctx)
	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return
		}
		m.logger.ErrorContext(ctx, "failed to acquire lease", slog.Any("error", err))
		return
	}

	wasHeld := m.isHeld.Load()
	m.isHeld.Store(acquired)

	if acquired {
		m.metrics.leaseHeld.Set(1)
		m.metrics.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	} else {
		m.metrics.leaseHeld.Set(0)
		m.metrics.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
	}

	if acquired && !wasHeld {
		m.logger.DebugContext(ctx, "lease acquired")
		if m.config.Callbacks.OnAcquired != nil {
			m.config.Callbacks.OnAcquired()
		}
	} else if !acquired && wasHeld {
		m.logger.DebugContext(ctx, "lease lost")
		if m.config.Callbacks.OnLost != nil {
			m.config.Callbacks.OnLost()
		}
	}
}

// tryRenew attempts to renew the lease.
func (m *LeaseManager) tryRenew(ctx context.Context) {
	renewed, err := m.renew(ctx)
	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return
		}
		m.logger.ErrorContext(ctx, "failed to renew lease", slog.Any("error", err))
		m.tryAcquire(ctx) // Try to re-acquire
		return
	}

	wasHeld := m.isHeld.Load()
	m.isHeld.Store(renewed)

	if renewed {
		m.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "success"}).Inc()
		m.logger.DebugContext(ctx, "lease renewed")
		if m.config.Callbacks.OnRenewed != nil {
			m.config.Callbacks.OnRenewed()
		}
	} else {
		m.metrics.leaseHeld.Set(0)
		m.metrics.operations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		if wasHeld {
			m.logger.DebugContext(ctx, "lease lost during renewal")
			if m.config.Callbacks.OnLost != nil {
				m.config.Callbacks.OnLost()
			}
		}
	}
}

// acquire attempts to acquire the lease using optimistic locking.
func (m *LeaseManager) acquire(ctx context.Context) (bool, error) {
	// Try to create the key
	_, err := m.ops.Create(ctx, m.config.Key, m.config.Value)
	if err == nil {
		return true, nil
	}

	if !errors.Is(err, jetstream.ErrKeyExists) {
		return false, err
	}

	// Key exists, check if we're the owner
	entry, err := m.ops.Get(ctx, m.config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Key was deleted, try again
			_, err = m.ops.Create(ctx, m.config.Key, m.config.Value)
			return err == nil, err
		}
		return false, err
	}

	if entry == nil || entry.Value() == nil {
		return false, nil
	}

	// Check if we're the owner
	if m.config.IsOwner != nil && m.config.IsOwner(entry.Value()) {
		// We own it, update to renew
		_, err := m.ops.Update(ctx, m.config.Key, m.config.Value, entry.Revision())
		return err == nil, err
	}

	return false, nil
}

// renew attempts to renew the lease.
func (m *LeaseManager) renew(ctx context.Context) (bool, error) {
	entry, err := m.ops.Get(ctx, m.config.Key)
	if err != nil {
		return false, err
	}

	if entry == nil || entry.Value() == nil {
		return false, ErrLeaseNotHeld
	}

	// Check ownership
	if m.config.IsOwner != nil && !m.config.IsOwner(entry.Value()) {
		return false, ErrLeaseNotHeld
	}

	_, err = m.ops.Update(ctx, m.config.Key, m.config.Value, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return false, ErrLeaseNotHeld
		}
		return false, err
	}

	return true, nil
}

// release releases the lease.
func (m *LeaseManager) release(ctx context.Context) {
	if !m.isHeld.Load() {
		return
	}

	entry, err := m.ops.Get(ctx, m.config.Key)
	if err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			m.logger.ErrorContext(ctx, "failed to get entry for release", slog.Any("error", err))
			m.metrics.operations.WithLabels(metrics.Labels{"op": "release", "result": "failure"}).Inc()
		}
		m.isHeld.Store(false)
		m.metrics.leaseHeld.Set(0)
		return
	}

	if entry == nil || entry.Value() == nil {
		m.isHeld.Store(false)
		m.metrics.leaseHeld.Set(0)
		return
	}

	// Only delete if we're the owner
	if m.config.IsOwner != nil && m.config.IsOwner(entry.Value()) {
		if err := m.ops.DeleteWithRevision(ctx, m.config.Key, entry.Revision()); err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				m.logger.ErrorContext(ctx, "failed to delete lease entry", slog.Any("error", err))
				m.metrics.operations.WithLabels(metrics.Labels{"op": "release", "result": "failure"}).Inc()
			}
		}
		if m.config.Callbacks.OnReleased != nil {
			m.config.Callbacks.OnReleased()
		}
	}

	m.isHeld.Store(false)
	m.metrics.leaseHeld.Set(0)
	m.metrics.operations.WithLabels(metrics.Labels{"op": "release", "result": "success"}).Inc()
}
