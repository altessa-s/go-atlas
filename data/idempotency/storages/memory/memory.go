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

// AttemptLock tries to acquire a lock for the given key.
func (s *Storage) AttemptLock(_ context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if e, exists := s.entries[key]; exists {
		// Check expiry
		if s.options.ttl > 0 && time.Now().After(e.expiresAt) {
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

	if s.options.ttl > 0 {
		e.expiresAt = now.Add(s.options.ttl)
	}

	s.entries[key] = e
	// Token is a defensive copy of the bytes we just wrote. Complete
	// will compare the current entry value against this token to
	// detect a stolen lock (TTL expiry → another AttemptLock won).
	return true, nil, slices.Clone(val), nil
}

// Complete marks the key as successfully processed.
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
	return nil
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
