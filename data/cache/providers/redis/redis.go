// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

// Provider is a Redis-backed cache provider.
// It supports key prefixing for namespace isolation.
type Provider struct {
	redisbase.Base
}

// New creates a new Redis provider with the given client and options.
// Panics if client is nil.
//
// Example:
//
//	p := redis.New(redisClient, redis.WithPrefix("myapp"))
func New(client redis.UniversalClient, opt ...Option) *Provider {
	options := newOptions(opt...)

	return &Provider{
		Base: redisbase.NewBase(client, options.prefix),
	}
}

// Delete removes a key from Redis.
func (p *Provider) Delete(ctx context.Context, key string) error {
	return redisutils.Del(ctx, p.Client(), p.Key(key))
}

// DeleteMany removes multiple keys from Redis in a single transaction.
func (p *Provider) DeleteMany(ctx context.Context, keys ...string) error {
	return p.Base.DeleteMany(ctx, keys...)
}

// Get retrieves a value from Redis by key.
// Returns ErrMissing if the key does not exist.
func (p *Provider) Get(ctx context.Context, key string) ([]byte, error) {
	return redisutils.GetBytes(ctx, p.Client(), p.Key(key), providers.ErrMissing)
}

// Save stores a value in Redis with the given TTL.
func (p *Provider) Save(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return redisutils.SetBytes(ctx, p.Client(), p.Key(key), value, ttl)
}

// Exists checks if a key exists in Redis.
func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	return p.Base.Exists(ctx, key)
}

var _ providers.Provider = (*Provider)(nil)
