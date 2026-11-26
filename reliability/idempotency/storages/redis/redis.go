// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/internal/redis/redisbase"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Keep the server timestamp and the opaque value in a single key so they
// share one TTL and one Redis Cluster slot. Zero denotes a completed entry.
const (
	completedPrefix = "0\n"
	lockAttempts    = 2
)

const luaTimestamp = `
local function nowMicros()
  local now = redis.call('TIME')
  return tonumber(now[1]) * 1000000 + tonumber(now[2])
end

local function lockValue(now, payload)
  return string.format('%.0f', now) .. '\n' .. payload
end
`

const luaRead = `
local function readEntry(key)
  local raw = redis.call('GET', key)
  if not raw then
    return nil, nil
  end
  local sep = string.find(raw, '\n', 1, true)
  local lockedAt = sep and tonumber(string.sub(raw, 1, sep - 1))
  if not lockedAt then
    error('invalid idempotency entry')
  end
  return lockedAt, string.sub(raw, sep + 1)
end
`

var acquireScript = redis.NewScript(luaTimestamp + `
if redis.call('SET', KEYS[1], lockValue(nowMicros(), ARGV[1]), 'NX', 'PX', ARGV[2]) then
  return 1
end
return 0
`)

var stealScript = redis.NewScript(luaRead + luaTimestamp + `
local lockedAt, current = readEntry(KEYS[1])
if not current then
  return {0, false}
end
local now = nowMicros()
local age = tonumber(ARGV[3])
if current ~= ARGV[1] or lockedAt == 0 or (age > 0 and now - lockedAt <= age) then
  return {0, current}
end
redis.call('SET', KEYS[1], lockValue(now, ARGV[2]), 'PX', ARGV[4])
return {1, false}
`)

var completeScript = redis.NewScript(luaRead + `
local _, current = readEntry(KEYS[1])
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

	redisKey := s.Key(key)
	for range lockAttempts {
		acquired, err := acquireScript.Run(ctx, s.Client(), []string{redisKey}, val, ttlMillis(ttl)).Int64()
		if err != nil {
			return false, nil, nil, coreerrs.WrapOperation(err, "attempt lock in Redis")
		}
		if acquired == 1 {
			return true, nil, slices.Clone(val), nil
		}
		data, err := s.Client().Get(ctx, redisKey).Bytes()
		if errors.Is(err, redis.Nil) {
			// Expiry or release raced SET NX. Retry acquisition once.
			continue
		}
		if err != nil {
			return false, nil, nil, coreerrs.WrapOperation(err, "get existing state from Redis")
		}
		_, current, ok := bytes.Cut(data, []byte{'\n'})
		if !ok {
			return false, nil, nil, errors.New("invalid idempotency entry in Redis")
		}
		return false, current, nil, nil
	}
	return false, nil, nil, nil
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

	data := append([]byte(completedPrefix), val...)
	if lockToken == nil {
		if err := s.Client().Set(ctx, s.Key(key), data, s.opts.ttl).Err(); err != nil {
			return coreerrs.WrapOperation(err, "complete idempotency key in Redis")
		}
		return nil
	}
	result, err := completeScript.Run(ctx, s.Client(), []string{s.Key(key)}, lockToken, data, ttlMillis(s.opts.ttl)).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "complete idempotency key in Redis")
	}
	if result != 1 {
		return storages.ErrLockStolen
	}
	return nil
}

// Steal checks the lock's age against Redis TIME and conditionally replaces
// the matching lock with the requested TTL in one Lua script.
func (s *Storage) Steal(ctx context.Context, key string, expected, replacement []byte, maxLockAge, ttl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}
	if ttl <= 0 {
		ttl = s.opts.ttl
	}
	age := maxLockAge.Microseconds()
	if maxLockAge > 0 {
		age = max(1, age)
	}
	result, err := stealScript.Run(ctx, s.Client(), []string{s.Key(key)}, expected, replacement, age, ttlMillis(ttl)).Slice()
	if err != nil {
		return false, nil, nil, coreerrs.WrapOperation(err, "steal idempotency key in Redis")
	}
	const resultFields = 2
	if len(result) == resultFields {
		switch result[0] {
		case int64(1):
			return true, nil, slices.Clone(replacement), nil
		case int64(0):
			if result[1] == nil {
				return false, nil, nil, nil
			}
			if current, ok := result[1].(string); ok {
				return false, []byte(current), nil, nil
			}
		}
	}
	return false, nil, nil, errors.New("unexpected steal result from Redis")
}

func ttlMillis(ttl time.Duration) int64 { return max(1, ttl.Milliseconds()) }

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

var releaseCASScript = redis.NewScript(luaRead + `
local _, current = readEntry(KEYS[1])
if current ~= ARGV[1] then return 0 end
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
