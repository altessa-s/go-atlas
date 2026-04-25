// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providers

import (
	"context"
	"errors"
	"time"
)

// Provider defines the interface for cache storage backends.
// Implementations must be safe for concurrent use.
type Provider interface {
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

// ErrMissing is returned when a cache key is not found.
var ErrMissing = errors.New("no cache found")
