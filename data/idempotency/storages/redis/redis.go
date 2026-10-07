// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/internal/redisbase"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// casScript is the Lua compare-and-set shared by [Storage.Complete] and
// [Storage.Steal]. Returns:
//
//	1  — current value matched the expected value; new value written
//	0  — current value differs (another holder owns the key)
//	-1 — key not found (TTL expired before we got here)
//
// KEYS[1] = redis key
// ARGV[1] = expected value (lockToken / expectedVal)
// ARGV[2] = new value
// ARGV[3] = TTL in milliseconds (positive integer)
var casScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  return -1
end
if current ~= ARGV[1] then
  return 0
end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

// Storage is a Redis-backed idempotency key store with TTL support.
// Uses atomic SET NX operations for thread-safe duplicate detection.
type Storage struct {
	redisbase.Base
	opts *options
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new Redis Storage with the given client.
// Panics if client is nil. Default TTL is 24 hours.
//
// Example:
//
//	storage := redis.New(rdb, redis.WithTTL(time.Hour))
func New(client redis.UniversalClient, opt ...Option) *Storage {
	opts := newOptions(opt...)

	return &Storage{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		opts: opts,
	}
}

// AttemptLock tries to acquire a lock for the given key using the
// backend's configured TTL.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return s.AttemptLockWithTTL(ctx, key, val, 0)
}

// AttemptLockWithTTL is like [Storage.AttemptLock] but lockTtl
// overrides the backend's configured TTL when positive.
func (s *Storage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	ttl := s.opts.ttl
	if lockTtl > 0 {
		ttl = lockTtl
	}

	// Try to set provided key with NX (only if not exists)
	ok, err := s.Client().SetNX(ctx, s.Key(key), val, ttl).Result()
	if err != nil {
		return false, nil, nil, coreerrs.WrapOperation(err, "attempt lock in Redis")
	}

	if ok {
		// Token is a defensive copy of the value we just wrote.
		// Complete will GET-and-compare via Lua to detect a stolen lock.
		return true, nil, slices.Clone(val), nil
	}

	// Key exists, get current state
	data, err := s.Client().Get(ctx, s.Key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Race: SetNX failed but Get says missing. Likely the key
			// expired between the two calls. Surface as "lock not
			// acquired, no existing data" — the caller can retry.
			return false, nil, nil, nil
		}
		return false, nil, nil, coreerrs.WrapOperation(err, "get existing state from Redis")
	}

	return false, data, nil, nil
}

// Complete marks the key as successfully processed using the
// backend's configured TTL.
//
// Uses a Lua script (GET + SET in one round-trip) to verify that
// the current value still matches lockToken before overwriting.
// Returns [storages.ErrLockStolen] when the lock has been taken
// over by another holder or has expired.
func (s *Storage) Complete(ctx context.Context, key string, val []byte, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	if lockToken == nil {
		// No CAS guard requested — fall back to unconditional overwrite.
		// Used by tests and adapters that bypass the safe path.
		if err := s.Client().Set(ctx, s.Key(key), val, s.opts.ttl).Err(); err != nil {
			return coreerrs.WrapOperation(err, "complete idempotency key in Redis")
		}
		return nil
	}

	return s.compareAndSet(ctx, key, lockToken, val, "complete idempotency key in Redis")
}

// Steal atomically replaces the value when current bytes equal
// expectedVal. Returns the new lockToken on success or
// [storages.ErrLockStolen] when expectedVal no longer matches.
func (s *Storage) Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	if key == "" {
		return nil, storages.ErrEmptyKey
	}

	if err := s.compareAndSet(ctx, key, expectedVal, newVal, "steal idempotency key in Redis"); err != nil {
		return nil, err
	}
	return slices.Clone(newVal), nil
}

// compareAndSet runs [casScript] for key with the backend TTL and maps
// its result: nil when the write happened, [storages.ErrLockStolen] when
// the key is missing or holds another value. op names the operation in
// transport errors.
func (s *Storage) compareAndSet(ctx context.Context, key string, expected, val []byte, op string) error {
	ttlMs := strconv.FormatInt(s.opts.ttl.Milliseconds(), 10)
	res, err := casScript.Run(ctx, s.Client(),
		[]string{s.Key(key)},
		expected, val, ttlMs,
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, op)
	}

	switch res {
	case 1:
		return nil
	case 0, -1:
		return storages.ErrLockStolen
	default:
		return coreerrs.Wrapf(storages.ErrLockStolen, "unexpected Lua result %d", res)
	}
}

// Delete removes the key from storage.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	if err := s.Client().Del(ctx, s.Key(key)).Err(); err != nil {
		return coreerrs.WrapOperation(err, "delete idempotency key from Redis")
	}

	return nil
}

var releaseCASScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
`)

// Release atomically removes key only while lockToken owns its entry.
func (s *Storage) Release(ctx context.Context, key string, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	result, err := releaseCASScript.Run(ctx, s.Client(), []string{s.Key(key)}, lockToken).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "release idempotency key in Redis")
	}
	if result != 1 {
		return storages.ErrLockStolen
	}
	return nil
}
