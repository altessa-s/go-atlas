// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisbase

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

// Base provides common functionality for Redis-based storage providers.
// It should be embedded in concrete provider implementations.
type Base struct {
	client redis.UniversalClient
	keys   *redisutils.KeyBuilder
}

// NewBase creates a new Base with the given client and key prefix.
// Panics if client is nil.
func NewBase(client redis.UniversalClient, keyPrefix string) Base {
	panics.MustNonNil(client, "redis client must not be nil")

	return Base{
		client: client,
		keys:   redisutils.NewKeyBuilder(keyPrefix),
	}
}

// Client returns the underlying Redis client.
func (b *Base) Client() redis.UniversalClient {
	return b.client
}

// Keys returns the KeyBuilder for building prefixed keys.
func (b *Base) Keys() *redisutils.KeyBuilder {
	return b.keys
}

// Key builds a single prefixed key.
func (b *Base) Key(key string) string {
	return b.keys.Build(key)
}

// BuildKeys builds multiple prefixed keys.
func (b *Base) BuildKeys(keys ...string) []string {
	return b.keys.BuildMany(keys)
}

// Exists checks if a key exists in Redis.
func (b *Base) Exists(ctx context.Context, key string) (bool, error) {
	val, err := b.client.Exists(ctx, b.Key(key)).Result()
	if err != nil || val == 0 {
		return false, err
	}
	return true, nil
}

// Delete removes a key from Redis.
func (b *Base) Delete(ctx context.Context, key string) error {
	return redisutils.Del(ctx, b.client, b.Key(key))
}

// DeleteMany removes multiple keys from Redis in a single transaction.
func (b *Base) DeleteMany(ctx context.Context, keys ...string) error {
	prefixedKeys := b.BuildKeys(keys...)

	_, err := b.client.TxPipelined(ctx, func(pl redis.Pipeliner) error {
		return pl.Del(ctx, prefixedKeys...).Err()
	})
	return err
}
