// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// entry holds an idempotency key with state and expiration metadata.
type entry struct {
	key       string
	value     []byte
	expiresAt time.Time
	lockedAt  time.Time
	locked    bool
}

// expired reports whether e's TTL has elapsed at now. Expiration is lazy, so
// an expired entry can still sit in the map until the next cleanup.
func (e *entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

// Storage is an in-memory idempotency key store with TTL support.
// It is safe for concurrent use. Use RunCleanup() to remove expired keys,
// either manually or via scheduler.
type Storage struct {
	entries     map[string]*entry
	mu          sync.RWMutex
	options     *options
	scheduler   schedtask.Registrar
	cleanupTask *cycle.Guard // Guards RunCleanup and marks scheduler management.
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new in-memory Storage with the provided options.
// Default TTL is 24 hours.
//
// If scheduler is provided and cleanupSchedule is set, cleanup is registered. A registration failure is returned.
// Otherwise, call RunCleanup() manually or register it externally.
//
// Example:
//
//	storage, err := memory.New(memory.WithTTL(time.Hour))
//	// With scheduler
//	storage, err := memory.New(
//	    memory.WithTTL(time.Hour),
//	    memory.WithScheduler(scheduler),
//	    memory.WithCleanupSchedule("@every 5m"),
//	)
func New(opt ...Option) (*Storage, error) {
	opts := newOptions(opt...)
	s := &Storage{
		options:   opts,
		entries:   make(map[string]*entry),
		scheduler: opts.scheduler,
	}
	s.cleanupTask = cycle.New(s.runCleanupCycle)

	if !nilcheck.IsNil(opts.scheduler) && opts.cleanupSchedule != "" {
		if err := s.registerCleanupTask(opts.cleanupSchedule); err != nil {
			return nil, coreerrs.WrapOperation(err, "register idempotency memory cleanup")
		}
	}
	return s, nil
}

// AttemptLock tries to acquire a lock for the given key using the
// backend's configured TTL.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return s.AttemptLockWithTTL(ctx, key, val, 0)
}

// AttemptLockWithTTL is like [Storage.AttemptLock] but lockTtl
// overrides the backend's configured TTL when positive.
func (s *Storage) AttemptLockWithTTL(_ context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	ttl := s.options.ttl
	if lockTtl > 0 {
		ttl = lockTtl
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.options.clock.Now()
	if e, exists := s.entries[key]; exists && !e.expired(now) {
		return false, slices.Clone(e.value), nil, nil
	}

	e := &entry{key: key, value: slices.Clone(val), lockedAt: now, locked: true}

	if ttl > 0 {
		e.expiresAt = now.Add(ttl)
	}

	s.entries[key] = e
	// Token is a defensive copy of the bytes we just wrote. Complete
	// will compare the current entry value against this token to
	// detect a stolen lock (TTL expiry → another AttemptLock won).
	return true, nil, slices.Clone(val), nil
}

// Complete marks the key as successfully processed using the
// backend's configured TTL.
//
// Returns [storages.ErrLockStolen] when the key has been taken over by
// another holder (lockToken doesn't match the current value, or the key
// has expired between AttemptLock and Complete).
func (s *Storage) Complete(_ context.Context, key string, val []byte, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.options.clock.Now()
	e, exists := s.entries[key]
	if !exists || e.expired(now) {
		// Key gone or expired — TTL elapsed between AttemptLock and
		// Complete, or someone Delete'd. Either way the lock isn't ours
		// anymore, and an expired entry must not be revived.
		return storages.ErrLockStolen
	}
	if lockToken != nil && !bytes.Equal(e.value, lockToken) {
		return storages.ErrLockStolen
	}

	e.value = slices.Clone(val)
	e.locked = false
	if s.options.ttl > 0 {
		e.expiresAt = now.Add(s.options.ttl)
	} else {
		e.expiresAt = time.Time{}
	}
	return nil
}

// Steal replaces an unexpired matching lock after the configured clock
// reports that it is old enough.
func (s *Storage) Steal(_ context.Context, key string, expected, replacement []byte, maxLockAge, ttl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.options.clock.Now()
	e, exists := s.entries[key]
	if !exists || e.expired(now) {
		return false, nil, nil, nil
	}
	if !e.locked || !bytes.Equal(e.value, expected) || (maxLockAge > 0 && now.Sub(e.lockedAt) <= maxLockAge) {
		return false, slices.Clone(e.value), nil, nil
	}

	e.value = slices.Clone(replacement)
	e.lockedAt = now
	e.expiresAt = time.Time{}
	if ttl <= 0 {
		ttl = s.options.ttl
	}
	if ttl > 0 {
		e.expiresAt = now.Add(ttl)
	}
	return true, nil, slices.Clone(replacement), nil
}

// Delete removes the key from storage.
func (s *Storage) Delete(_ context.Context, key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
	return nil
}

// Close releases resources.
// No-op since there are no background goroutines to stop.
func (s *Storage) Close() error {
	return nil
}

// Release atomically removes key only while lockToken owns its unexpired entry.
func (s *Storage) Release(_ context.Context, key string, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || !bytes.Equal(e.value, lockToken) || e.expired(s.options.clock.Now()) {
		return storages.ErrLockStolen
	}
	delete(s.entries, key)
	return nil
}
