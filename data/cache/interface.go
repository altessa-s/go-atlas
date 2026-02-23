// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"time"
)

// Cacher defines the interface for basic cache operations.
// Implementations must be safe for concurrent use.
type Cacher interface {
	// Save stores a value with the given key.
	Save(ctx context.Context, key string, value any, ttl ...time.Duration) error

	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)

	// Get retrieves a value by key.
	Get(ctx context.Context, key string, value any) error

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// DeleteMany removes multiple keys from the cache.
	DeleteMany(ctx context.Context, key ...string) error
}

// FallbackCacher defines the interface for cache operations including fallback support.
// It inherits all methods from Cacher and adds GetWithFallback.
type FallbackCacher interface {
	Cacher

	// GetWithFallback retrieves a value or calls fallback if not found.
	GetWithFallback(ctx context.Context, key string, value any, fallback Fallback) error
}
