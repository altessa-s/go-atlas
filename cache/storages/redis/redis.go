// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/storages"
	"github.com/altessa-s/go-atlas/internal/redis/redisbase"
	"github.com/altessa-s/go-atlas/internal/redis/redisutils"
)

// Storage is a Redis-backed cache provider.
// It supports key prefixing for namespace isolation.
type Storage struct {
	redisbase.Base
}

// New creates a new Redis provider with the given client and options.
// Panics if client is nil.
//
// Example:
//
//	p := redis.New(redisClient, redis.WithPrefix("myapp"))
func New(client redis.UniversalClient, opt ...Option) *Storage {
	options := newOptions(opt...)

	return &Storage{
		Base: redisbase.NewBase(client, options.prefix),
	}
}

// Delete removes a key from Redis.
func (p *Storage) Delete(ctx context.Context, key string) error {
	return redisutils.Del(ctx, p.Client(), p.Key(key))
}

var compareAndDeleteScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

// CompareAndDelete implements [storages.CompareDeleter] in one Redis script.
func (p *Storage) CompareAndDelete(ctx context.Context, key string, expected []byte) (bool, error) {
	deleted, err := compareAndDeleteScript.Run(ctx, p.Client(), []string{p.Key(key)}, expected).Int64()
	return deleted == 1, err
}

// DeleteMany removes multiple keys from Redis in one pipeline of single-key commands.
func (p *Storage) DeleteMany(ctx context.Context, keys ...string) error {
	return p.Base.DeleteMany(ctx, keys...)
}

// Get retrieves a value from Redis by key.
// Returns ErrMissing if the key does not exist.
func (p *Storage) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := p.Client().Get(ctx, p.Key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, storages.ErrMissing
	}
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

// GetMany retrieves several values in one round trip. It sends a pipeline of
// GET commands rather than MGET, so keys that hash to different Redis Cluster
// slots work. Missing keys are absent from the returned map.
func (p *Storage) GetMany(ctx context.Context, keys []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	pipe := p.Client().Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, p.Key(key))
	}
	// Exec reports the first failed command, which is redis.Nil for a missing
	// key; the per-command results below tell the two apart.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return nil, err
		}
		out[keys[i]] = []byte(val)
	}
	return out, nil
}

// Save stores a value in Redis with the given TTL.
func (p *Storage) Save(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return redisutils.SetBytes(ctx, p.Client(), p.Key(key), value, ttl)
}

// Exists checks if a key exists in Redis.
func (p *Storage) Exists(ctx context.Context, key string) (bool, error) {
	return p.Base.Exists(ctx, key)
}

var (
	_ storages.Storage        = (*Storage)(nil)
	_ storages.MultiGetter    = (*Storage)(nil)
	_ storages.CompareDeleter = (*Storage)(nil)
)
