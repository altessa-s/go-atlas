// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/uniq/providers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	goredis "github.com/redis/go-redis/v9"
)

// Provider implements the uniq.Provider interface using Redis Key-Value store.
// It provides thread-safe operations for managing unique values with support
// for TTL and key prefixes.
type Provider struct {
	redisbase.Base
	opts *options
}

var (
	_ providers.Provider = (*Provider)(nil)
	_ providers.Prober   = (*Provider)(nil)
)

// New creates a new Redis provider with the specified client, prefix, and TTL.
// Panics if client is nil.
func New(client goredis.UniversalClient, opt ...Option) *Provider {
	opts := newOptions(opt...)

	return &Provider{
		Base: redisbase.NewBase(client, opts.prefix),
		opts: opts,
	}
}

// Add adds a key to the Redis store with the configured TTL.
// If the key already exists, it will be overwritten with the new TTL.
func (p *Provider) Add(ctx context.Context, key string) error {
	return p.AddWithValue(ctx, key, []byte("1"))
}

// AddWithValue adds a key with an associated value to the Redis store.
func (p *Provider) AddWithValue(ctx context.Context, key string, value []byte) error {
	err := p.Client().Set(ctx, p.Key(key), value, p.opts.ttl).Err()
	if err != nil {
		return coreerrs.WrapOperation(err, "add key")
	}
	return nil
}

// Exist checks if a key exists in the Redis store.
// Returns true if the key exists and hasn't expired, false otherwise.
func (p *Provider) Exist(ctx context.Context, key string) (bool, error) {
	exists, err := p.Client().Exists(ctx, p.Key(key)).Result()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "check key existence")
	}
	return exists > 0, nil
}

// Remove removes a key from the Redis store.
// If the key doesn't exist, no error is returned.
func (p *Provider) Remove(ctx context.Context, key string) error {
	err := p.Client().Del(ctx, p.Key(key)).Err()
	if err != nil {
		return coreerrs.WrapOperation(err, "remove key")
	}
	return nil
}

// GetValue retrieves the value associated with a key in the Redis store.
func (p *Provider) GetValue(ctx context.Context, key string) ([]byte, error) {
	value, err := p.Client().Get(ctx, p.Key(key)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil // Key does not exist
		}
		return nil, coreerrs.Wrapf(err, "failed to get value for key %s", key)
	}
	return []byte(value), nil
}

// clearScript is a Lua script that atomically deletes keys in batches using SCAN
const clearScript = `
local pattern = ARGV[1]
local cursor = "0"
local deleted = 0

repeat
    local res = redis.call('SCAN', cursor, 'MATCH', pattern, 'COUNT', 100)
    cursor = res[1]
    local keys = res[2]

    if #keys > 0 then
        deleted = deleted + redis.call('DEL', unpack(keys))
    end
until cursor == "0"

return deleted
`

// Clear removes all keys from the Redis store atomically using a Lua script.
// The operation uses SCAN to avoid blocking the Redis server and is performed
// in batches of 100 keys.
func (p *Provider) Clear(ctx context.Context) error {
	// Execute Lua script to atomically delete all keys with prefix in batches
	_, err := p.Client().Eval(ctx, clearScript, []string{}, p.Keys().Pattern()).Result()
	if err != nil {
		return coreerrs.WrapOperation(err, "clear keys")
	}
	return nil
}

// Probe implements [providers.Prober]. Returns nil when the Redis
// client responds to PING; any error surfaces as an unhealthy probe
// (typically caused by a closed client or unreachable Redis).
func (p *Provider) Probe(ctx context.Context) error {
	if err := p.Client().Ping(ctx).Err(); err != nil {
		return coreerrs.Wrap(err, "redis ping failed")
	}
	return nil
}
