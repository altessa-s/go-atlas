// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"errors"
	"time"
)

// Storage defines the interface for cache storage backends.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Save stores a value with the given key and TTL. The value is borrowed
	// for the call. An implementation that retains it must take a copy.
	Save(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Get retrieves an owned copy of the value by key. Returns ErrMissing if
	// not found.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// DeleteMany removes multiple keys from the cache.
	DeleteMany(ctx context.Context, keys ...string) error
}

// ExistenceChecker is an optional [Storage] capability for direct key lookups.
type ExistenceChecker interface {
	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
}

// MultiGetter is an optional interface a [Storage] implements to read many
// keys in one round trip. Callers type-assert for it and fall back to a loop
// of Get calls when it is absent. Implementations must be safe for
// concurrent use.
type MultiGetter interface {
	// GetMany returns owned copies of the values that are present. Missing keys
	// are absent from the map. A missing key is not an error.
	GetMany(ctx context.Context, keys []string) (map[string][]byte, error)
}

// CompareDeleter is an optional storage capability used for fail-open decode
// cleanup. A storage without it leaves the unreadable value to expire or be
// replaced by a later write.
type CompareDeleter interface {
	// CompareAndDelete atomically deletes key if present and its stored value
	// matches expected byte for byte. It reports whether it deleted the value.
	CompareAndDelete(ctx context.Context, key string, expected []byte) (deleted bool, err error)
}

// InProcess is an optional interface a [Storage] implements to report that
// it serves every call from process memory without blocking on I/O. The cache
// then does not derive its default operation timeout for a caller context
// without a deadline, saving a timer per call on the hit path.
type InProcess interface {
	// InProcess reports whether every call is served from process memory.
	InProcess() bool
}

// ErrMissing is returned when a cache key is not found.
var ErrMissing = errors.New("no cache found")
