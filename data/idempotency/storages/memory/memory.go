// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// entry holds an idempotency key with state and expiration metadata.
type entry struct {
	key       string
	value     []byte
	createdAt time.Time
	expiresAt time.Time
}

// Storage is an in-memory idempotency key store with TTL support.
// It is safe for concurrent use. Use RunCleanup() to remove expired keys,
// either manually or via scheduler.
type Storage struct {
	entries        map[string]*entry
	mu             sync.RWMutex
	options        *options
	scheduler      corescheduler.TaskRegistrar
	cleanupRunning atomic.Bool // Guards against concurrent RunCleanup calls.

	schedulerCleanupRegistered atomic.Bool // Marks if RunCleanup is managed by scheduler.
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new in-memory Storage with the provided options.
// Default TTL is 24 hours.
//
// If scheduler is provided and cleanupSchedule is set, cleanup task will be registered.
// Otherwise, call RunCleanup() manually or register it externally.
//
// Example:
//
//	storage := memory.New(memory.WithTTL(time.Hour))
//	// With scheduler
//	storage := memory.New(
//	    memory.WithTTL(time.Hour),
//	    memory.WithScheduler(scheduler),
//	    memory.WithCleanupSchedule("@every 5m"),
//	)
func New(opt ...Option) *Storage {
	opts := newOptions(opt...)
	s := &Storage{
		options:   opts,
		entries:   make(map[string]*entry),
		scheduler: opts.scheduler,
	}

	// Register cleanup task with scheduler if provided
	_ = s.registerCleanupTask(opts.cleanupSchedule) //nolint:errcheck // task registration is optional

	return s
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

	if e, exists := s.entries[key]; exists {
		// Check expiry against the entry's own expiresAt — set when it
		// was created with whatever TTL was active at the time.
		if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
			delete(s.entries, key)
		} else {
			return false, e.value, nil, nil
		}
	}

	now := time.Now()
	e := &entry{
		key:       key,
		value:     val,
		createdAt: now,
	}

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

	e, exists := s.entries[key]
	if !exists {
		// Key gone — TTL expired between AttemptLock and Complete, or
		// someone Delete'd. Either way the lock isn't ours anymore.
		return storages.ErrLockStolen
	}
	if lockToken != nil && !bytes.Equal(e.value, lockToken) {
		return storages.ErrLockStolen
	}

	e.value = val
	if s.options.ttl > 0 {
		e.expiresAt = time.Now().Add(s.options.ttl)
	} else {
		e.expiresAt = time.Time{}
	}
	return nil
}

// Steal atomically replaces the value when current bytes equal
// expectedVal. Returns the new lockToken on success or
// [storages.ErrLockStolen] when expectedVal no longer matches.
func (s *Storage) Steal(_ context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	if key == "" {
		return nil, storages.ErrEmptyKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e, exists := s.entries[key]
	if !exists {
		return nil, storages.ErrLockStolen
	}
	if !bytes.Equal(e.value, expectedVal) {
		return nil, storages.ErrLockStolen
	}

	e.value = newVal
	if s.options.ttl > 0 {
		e.expiresAt = time.Now().Add(s.options.ttl)
	}
	return slices.Clone(newVal), nil
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
