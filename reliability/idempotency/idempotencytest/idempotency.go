// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencytest

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

// MockIdempotencyStorage is a concurrency-safe, in-memory implementation of
// reliability/idempotency/storages.Storage for testing.
//
// [MockIdempotencyStorage.AttemptLock] returns false and the existing value
// when the key is already present, mimicking real lock-or-load semantics.
type MockIdempotencyStorage struct {
	mu      sync.Mutex
	entries map[string][]byte
}

// NewMockIdempotencyStorage returns a ready-to-use [MockIdempotencyStorage]
// with an empty entry set.
func NewMockIdempotencyStorage() *MockIdempotencyStorage {
	return &MockIdempotencyStorage{entries: make(map[string][]byte)}
}

// AttemptLock stores val under key if the key does not yet exist
// (returns true, nil, lockToken, nil). The lockToken is a copy of the
// stored value, matching the real memory backend's CAS contract. If
// the key already exists it returns (false, existingVal, nil, nil).
func (m *MockIdempotencyStorage) AttemptLock(_ context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.entries[key]; ok {
		return false, existing, nil, nil
	}
	m.entries[key] = val
	return true, nil, slices.Clone(val), nil
}

// Complete overwrites the value stored under key when lockToken still
// matches the current value (CAS). Returns
// [storages.ErrLockStolen] when the lock has been taken over by another
// holder. Pass lockToken=nil to bypass the guard for legacy tests.
func (m *MockIdempotencyStorage) Complete(_ context.Context, key string, val []byte, lockToken []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.entries[key]
	if !exists {
		return storages.ErrLockStolen
	}
	if lockToken != nil && !bytes.Equal(current, lockToken) {
		return storages.ErrLockStolen
	}
	m.entries[key] = val
	return nil
}

// AttemptLockWithTTL ignores lockTtl and delegates to AttemptLock —
// the mock doesn't model TTL. Tests that need real TTL behavior
// should use a real backend.
func (m *MockIdempotencyStorage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, _ time.Duration) (bool, []byte, []byte, error) {
	return m.AttemptLock(ctx, key, val)
}

// UsesKeeperClock lets tests forge the holder's wire timestamp.
func (m *MockIdempotencyStorage) UsesKeeperClock() bool { return true }

// Steal replaces the matching value after the Keeper checks its age.
// The mock omits TTL handling.
func (m *MockIdempotencyStorage) Steal(_ context.Context, key string, expected, replacement []byte, _, _ time.Duration) (bool, []byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.entries[key]
	if !exists || !bytes.Equal(current, expected) {
		return false, slices.Clone(current), nil, nil
	}
	m.entries[key] = replacement
	return true, nil, slices.Clone(replacement), nil
}

// SetEntry overwrites the raw bytes stored under key without any CAS
// check. Tests use this to inject a forged InProgress wire (e.g. with
// LockedAt set in the past) to exercise the orphan-steal path.
func (m *MockIdempotencyStorage) SetEntry(key string, val []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = slices.Clone(val)
}

// Entry returns a copy of the bytes stored under key. The second
// return value is false when the key is absent. Used by tests that
// need to inspect or forge wire-level state.
func (m *MockIdempotencyStorage) Entry(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.entries[key]
	if !ok {
		return nil, false
	}
	return slices.Clone(val), true
}

// Delete removes the entry for key.
func (m *MockIdempotencyStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

// Release removes key only if the caller still owns its value.
func (m *MockIdempotencyStorage) Release(_ context.Context, key string, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.entries[key]
	if !ok || !bytes.Equal(current, lockToken) {
		return storages.ErrLockStolen
	}
	delete(m.entries, key)
	return nil
}
