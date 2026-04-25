// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kvstore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/altessa-s/go-atlas/data/mongo"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrKeyNotFound is returned by Backend.Get when the key doesn't exist.
// The JSONStorage adapter maps this to mongo.ErrCursorNotFound.
var ErrKeyNotFound = errors.New("key not found")

// Backend is the interface that key-value storage backends must implement.
// This abstracts away the specifics of each storage system (Redis, NATS, etc.)
// and provides a simple get/set/delete interface.
//
// Implementations should:
//   - Return ErrKeyNotFound from Get when key doesn't exist or has expired
//   - Handle TTL internally (either via backend config or per-operation)
//   - Be thread-safe for concurrent access
type Backend interface {
	// Get retrieves a value by key.
	// Returns ErrKeyNotFound if the key doesn't exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with the given key.
	// TTL handling is implementation-specific.
	Set(ctx context.Context, key string, value []byte) error

	// Delete removes a key from storage.
	// Should be idempotent - return nil if key doesn't exist.
	Delete(ctx context.Context, key string) error
}

// JSONStorage wraps a Backend to provide automatic JSON serialization
// for cursor metadata. It implements mongo.CursorStorage.
//
// This eliminates duplicated JSON marshal/unmarshal logic across
// different storage backends.
type JSONStorage struct {
	backend Backend
	name    string // Backend name for error messages (e.g., "redis", "nats")
}

// NewJSONStorage creates a new JSONStorage adapter wrapping the given backend.
//
// Parameters:
//   - backend: The underlying key-value storage backend
//   - name: Human-readable name for error messages (e.g., "redis", "nats")
//
// Example:
//
//	backend := natsbackend.New(kv)
//	storage := kvstore.NewJSONStorage(backend, "nats")
func NewJSONStorage(backend Backend, name string) *JSONStorage {
	return &JSONStorage{
		backend: backend,
		name:    name,
	}
}

// Store saves cursor metadata to the backend as JSON.
// The metadata is serialized to JSON before storage.
func (s *JSONStorage) Store(ctx context.Context, key string, metadata *mongo.CursorMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal cursor metadata")
	}

	if err := s.backend.Set(ctx, key, data); err != nil {
		return coreerrs.Wrapf(err, "failed to store cursor in %s", s.name)
	}

	return nil
}

// Load retrieves cursor metadata from the backend.
// Returns mongo.ErrCursorNotFound if the key doesn't exist or has expired.
func (s *JSONStorage) Load(ctx context.Context, key string) (*mongo.CursorMetadata, error) {
	data, err := s.backend.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, mongo.ErrCursorNotFound
		}
		return nil, coreerrs.Wrapf(err, "failed to load cursor from %s", s.name)
	}

	var metadata mongo.CursorMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, coreerrs.WrapOperation(err, "unmarshal cursor metadata")
	}

	return &metadata, nil
}

// Delete removes cursor metadata from the backend.
// Returns nil if key doesn't exist (idempotent).
func (s *JSONStorage) Delete(ctx context.Context, key string) error {
	if err := s.backend.Delete(ctx, key); err != nil {
		return coreerrs.Wrapf(err, "failed to delete cursor from %s", s.name)
	}
	return nil
}
