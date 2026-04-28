// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
)

// MockNetError implements the [net.Error] interface for testing.
// The exported fields control the return values of the corresponding methods:
// Msg is returned by Error, IsTimeout by Timeout, and IsTemp by Temporary.
type MockNetError struct {
	IsTimeout bool
	IsTemp    bool
	Msg       string
}

func (e *MockNetError) Error() string   { return e.Msg }
func (e *MockNetError) Timeout() bool   { return e.IsTimeout }
func (e *MockNetError) Temporary() bool { return e.IsTemp }

// --- MockUniqProvider ---

// MockUniqProvider is a concurrency-safe, in-memory implementation of
// data/uniq/providers.Provider for testing.
//
// Set AddErr to make [MockUniqProvider.Add] and [MockUniqProvider.AddWithValue]
// return that error instead of storing the key.
type MockUniqProvider struct {
	mu     sync.Mutex
	keys   map[string][]byte
	AddErr error
}

// NewMockUniqProvider returns a ready-to-use [MockUniqProvider] with an empty key set.
func NewMockUniqProvider() *MockUniqProvider {
	return &MockUniqProvider{keys: make(map[string][]byte)}
}

// Add registers key with a nil value. It returns AddErr if set.
func (m *MockUniqProvider) Add(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.AddErr != nil {
		return m.AddErr
	}
	m.keys[key] = nil
	return nil
}

// AddWithValue registers key with the given value. It returns AddErr if set.
func (m *MockUniqProvider) AddWithValue(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.AddErr != nil {
		return m.AddErr
	}
	m.keys[key] = value
	return nil
}

// Exist reports whether key has been registered.
func (m *MockUniqProvider) Exist(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.keys[key]
	return ok, nil
}

// GetValue returns the value associated with key, or nil if the key does not exist.
func (m *MockUniqProvider) GetValue(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.keys[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

// Remove deletes key from the store.
func (m *MockUniqProvider) Remove(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, key)
	return nil
}

// Clear removes all keys from the store.
func (m *MockUniqProvider) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys = make(map[string][]byte)
	return nil
}

// --- MockIdempotencyStorage ---

// MockIdempotencyStorage is a concurrency-safe, in-memory implementation of
// data/idempotency/storages.Storage for testing.
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

// CompleteWithTTL ignores resultTtl and delegates to Complete — the
// mock doesn't model TTL.
func (m *MockIdempotencyStorage) CompleteWithTTL(ctx context.Context, key string, val []byte, lockToken []byte, _ time.Duration) error {
	return m.Complete(ctx, key, val, lockToken)
}

// Delete removes the entry for key.
func (m *MockIdempotencyStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

// SupportsAttemptLockWithTTL reports true. The mock accepts any TTL
// on AttemptLockWithTTL (silently ignored).
func (m *MockIdempotencyStorage) SupportsAttemptLockWithTTL() bool { return true }

// SupportsCompleteWithTTL reports true. The mock accepts any TTL on
// CompleteWithTTL (silently ignored). Tests that care about the
// false-case (NATS-style) should construct a Storage that returns
// false explicitly.
func (m *MockIdempotencyStorage) SupportsCompleteWithTTL() bool { return true }
