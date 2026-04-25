// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/internal/redisutils"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"
)

// backend implements kvstore.Backend for Redis.
type backend struct {
	redisbase.Base
	ttl time.Duration
}

// Get retrieves a value from Redis.
func (b *backend) Get(ctx context.Context, key string) ([]byte, error) {
	return redisutils.GetBytes(ctx, b.Client(), b.Key(key), kvstore.ErrKeyNotFound)
}

// Set stores a value in Redis with TTL.
func (b *backend) Set(ctx context.Context, key string, value []byte) error {
	return redisutils.SetBytes(ctx, b.Client(), b.Key(key), value, b.ttl)
}

// Delete removes a key from Redis (idempotent).
func (b *backend) Delete(ctx context.Context, key string) error {
	return redisutils.Del(ctx, b.Client(), b.Key(key))
}

// Storage is a Redis implementation of mongox.CursorStorage.
// This implementation is suitable for production deployments with multiple instances
// where cursor sharing across instances is required.
//
// Features:
//   - Distributed: Cursors shared across all application instances
//   - Automatic expiration: Redis TTL handles cursor cleanup
//   - Persistent: Survives application restarts (if Redis is persistent)
//   - Thread-safe: Redis handles concurrent access
//   - Configurable key prefix: Prevents key collisions
//
// Example usage:
//
//	rdb := redis.NewClient(&redis.Options{
//	    Addr: "localhost:6379",
//	})
//
//	storage := redis.New(rdb, redis.WithTTL(1*time.Hour), redis.WithKeyPrefix("cursors:"))
//
//	result, err := mongox.ListCursor(ctx, collection,
//	    mongox.WithListCursorStorage(storage),
//	    mongox.WithListCursorLimit(50),
//	)
type Storage struct {
	*kvstore.JSONStorage
}

// New creates a new Redis cursor storage with the specified options.
//
// The client parameter can be:
//   - *redis.Client: Single Redis instance
//   - *redis.ClusterClient: Redis Cluster
//   - *redis.Ring: Redis Ring
//   - *redis.SentinelClient: Redis Sentinel
//
// All types implement redis.UniversalClient interface.
//
// Parameters:
//   - client: Redis client (any implementation of redis.UniversalClient)
//   - opts: Optional configuration functions
//
// Returns:
//   - *Storage: Ready to use storage instance
//
// Example:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redis.New(rdb, redis.WithTTL(1*time.Hour))
func New(client redis.UniversalClient, opts ...Option) *Storage {
	options := newOptions(opts...)

	// Normalize invalid values to defaults to keep New() non-panicking.
	if options.ttl <= 0 {
		options.ttl = DefaultTTL
	}
	if options.keyPrefix == "" {
		options.keyPrefix = DefaultKeyPrefix
	}

	// Create the JSON storage adapter with our backend
	return &Storage{
		JSONStorage: kvstore.NewJSONStorage(&backend{
			Base: redisbase.NewBase(client, options.keyPrefix),
			ttl:  options.ttl,
		}, "redis"),
	}
}
