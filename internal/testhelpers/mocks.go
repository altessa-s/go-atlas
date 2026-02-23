// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrMockMissing is a sentinel returned by [MockCacheProvider.Get] when a key
// is not present in the in-memory store. Tests should match against this value
// the same way they would match providers.ErrMissing in production code.
var ErrMockMissing = errors.New("no cache found")

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

// --- MockCacheProvider ---

// MockCacheProvider is a concurrency-safe, in-memory implementation of
// data/cache/providers.Provider for testing.
//
// Error injection: set SaveErr, GetErr, DeleteErr, or ExistsErr to make the
// corresponding method return that error instead of performing its normal operation.
// DeleteMany reuses DeleteErr.
//
// Call counting: each method increments its respective *Calls counter on every
// invocation, regardless of whether an error is injected.
type MockCacheProvider struct {
	mu        sync.Mutex
	store     map[string][]byte
	SaveErr   error
	GetErr    error
	DeleteErr error
	ExistsErr error

	SaveCalls       int
	GetCalls        int
	DeleteCalls     int
	DeleteManyCalls int
	ExistsCalls     int
}

// NewMockCacheProvider returns a ready-to-use [MockCacheProvider] with an
// empty store and all error injection fields set to nil.
func NewMockCacheProvider() *MockCacheProvider {
	return &MockCacheProvider{store: make(map[string][]byte)}
}

// Save stores value under key. It returns SaveErr if set.
func (m *MockCacheProvider) Save(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SaveCalls++
	if m.SaveErr != nil {
		return m.SaveErr
	}
	m.store[key] = value
	return nil
}

// Get returns the value for key, or [ErrMockMissing] if the key does not exist.
// It returns GetErr if set.
func (m *MockCacheProvider) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCalls++
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	v, ok := m.store[key]
	if !ok {
		return nil, ErrMockMissing
	}
	return v, nil
}

// Delete removes key from the store. It returns DeleteErr if set.
func (m *MockCacheProvider) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteCalls++
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	delete(m.store, key)
	return nil
}

// DeleteMany removes all provided keys from the store. It returns DeleteErr if set.
func (m *MockCacheProvider) DeleteMany(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteManyCalls++
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	for _, k := range keys {
		delete(m.store, k)
	}
	return nil
}

// Exists reports whether key is present in the store. It returns ExistsErr if set.
func (m *MockCacheProvider) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ExistsCalls++
	if m.ExistsErr != nil {
		return false, m.ExistsErr
	}
	_, ok := m.store[key]
	return ok, nil
}

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

// AttemptLock stores val under key if the key does not yet exist (returns true, nil, nil).
// If the key already exists it returns false and the previously stored value.
func (m *MockIdempotencyStorage) AttemptLock(_ context.Context, key string, val []byte) (bool, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.entries[key]; ok {
		return false, existing, nil
	}
	m.entries[key] = val
	return true, nil, nil
}

// Complete overwrites the value stored under key, marking the operation as finished.
func (m *MockIdempotencyStorage) Complete(_ context.Context, key string, val []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = val
	return nil
}

// Delete removes the entry for key.
func (m *MockIdempotencyStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}
