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
	// Save stores a value with the given key and TTL.
	Save(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Get retrieves a value by key. Returns ErrMissing if not found.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// DeleteMany removes multiple keys from the cache.
	DeleteMany(ctx context.Context, keys ...string) error

	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
}

// MultiGetter is an optional interface a [Storage] implements to read many
// keys in one round trip. Callers type-assert for it and fall back to a loop
// of Get calls when it is absent. Implementations must be safe for
// concurrent use.
type MultiGetter interface {
	// GetMany returns the values of the keys that are present. Missing keys
	// are absent from the map; a missing key is not an error.
	GetMany(ctx context.Context, keys []string) (map[string][]byte, error)
}

// ErrMissing is returned when a cache key is not found.
var ErrMissing = errors.New("no cache found")
