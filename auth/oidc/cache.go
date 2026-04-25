// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"time"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

const (
	// DefaultTokensCacheKeyPrefix is the default prefix for validated token cache keys.
	DefaultTokensCacheKeyPrefix = "tokens:"

	// DefaultRevokedTokensCacheKeyPrefix is the default prefix for revoked token cache keys.
	DefaultRevokedTokensCacheKeyPrefix = "revoked-tokens:"

	// DefaultActiveTokensCacheKeyPrefix is the default prefix for active introspection cache keys.
	DefaultActiveTokensCacheKeyPrefix = "active-tokens:"
)

// Cacher defines the interface for caching validated tokens.
// Implementations must be thread-safe. Used to avoid redundant token validation.
//
// Example:
//
//	cache.Save(ctx, "key", claims, 5*time.Minute)
//	cache.Get(ctx, "key", &claims)
type Cacher interface {
	// Save stores a value in the cache with an optional TTL.
	Save(ctx context.Context, key string, value any, ttl ...time.Duration) error

	// Get retrieves a value from the cache by key into a pointer.
	Get(ctx context.Context, key string, value any) error
}

// tokenCacheKey generates a SHA-256 hash-based cache key from a token string.
// The prefix is prepended to the hex-encoded hash.
func tokenCacheKey(prefix, token string) string {
	return corehash.SHA256HexWithPrefix(prefix, token)
}
