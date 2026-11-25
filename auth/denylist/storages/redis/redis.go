// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"
)

// revokedValue is the sentinel stored at a revocation key; only its presence
// matters.
const revokedValue = "1"

// ErrUnsupportedClient indicates that the client cannot support the requested read or scan.
var ErrUnsupportedClient = errors.New("unsupported redis client for denylist operation")

// Store is a Redis-backed distributed exact revocation store. Each revoked key
// is a single Redis string keyed as keyPrefix+key with value "1"; Redis expires
// bounded revocations natively via per-key TTL.
//
// The caller owns the Redis client. Store is safe for concurrent use.
type Store struct {
	client redis.UniversalClient
	opts   *options
}

// ErrReplicaReadNotAllowed indicates that a strict read could not verify a primary.
var ErrReplicaReadNotAllowed = errors.New("denylist read requires a Redis primary")

// Driver errors can echo credentials and command arguments. Expose only the operation
// while allowing callers to match the original cause with errors.Is.
type operationError struct {
	op    string
	cause error
}

func (e operationError) Error() string        { return "redis denylist: " + e.op }
func (e operationError) Is(target error) bool { return errors.Is(e.cause, target) }

// New returns a Store backed by client. The client is required; pass
// [WithKeyPrefix] to override the default key namespace.
func New(client redis.UniversalClient, opts ...Option) (*Store, error) {
	o := newOptions(opts...)
	if o.strictExactReads {
		if err := verifyStrictClient(client); err != nil {
			return nil, err
		}
	}
	return &Store{client: client, opts: o}, nil
}

func verifyStrictClient(client redis.UniversalClient) error {
	switch c := client.(type) {
	case *redis.Client:
		if c == nil || c.Options().ClientSideCacheConfig != nil || c.Options().ClientSideCache != nil {
			return ErrUnsupportedClient
		}
	case *redis.ClusterClient:
		if c == nil {
			return ErrUnsupportedClient
		}
		o := c.Options()
		if o.ReadOnly || o.RouteByLatency || o.RouteRandomly {
			return ErrReplicaReadNotAllowed
		}
		// A custom node factory can enable caching independently of the cluster options.
		if o.ClientSideCacheConfig != nil || reflect.ValueOf(o.NewClient).Pointer() != reflect.ValueOf(redis.NewClient).Pointer() {
			return ErrUnsupportedClient
		}
	default:
		return ErrUnsupportedClient
	}
	return nil
}

// redisKey returns the namespaced Redis key for a bare revocation key.
func (s *Store) redisKey(key string) string {
	return s.opts.keyPrefix + key
}

// IsRevoked reports, authoritatively, whether key is revoked. A bounded
// revocation whose TTL has elapsed is gone from Redis and reports false.
func (s *Store) IsRevoked(ctx context.Context, key string) (bool, error) {
	if s.opts.strictExactReads {
		n, err := s.client.EvalRO(redis.WithoutCache(ctx), strictReadScript, []string{s.redisKey(key)}).Int64()
		if err != nil {
			return false, operationError{"check revocation", err}
		}
		if n != 0 && n != 1 {
			return false, ErrReplicaReadNotAllowed
		}
		return n == 1, nil
	}
	n, err := s.client.Exists(ctx, s.redisKey(key)).Result()
	if err != nil {
		return false, operationError{"check revocation", err}
	}
	return n > 0, nil
}

// Revoke denies key permanently, until a matching [Store.Restore]. The key is
// written with no expiry.
func (s *Store) Revoke(ctx context.Context, key string) error {
	if err := s.client.Set(ctx, s.redisKey(key), revokedValue, 0).Err(); err != nil {
		return operationError{"revoke", err}
	}
	return nil
}

// RevokeUntil extends a bounded revocation to at least ttl, rounded up to a
// millisecond. A permanent revocation stays permanent. A non-positive
// ttl is a no-op — the token is already invalid on its own — mirroring the
// in-memory denylist's non-positive expiry.
func (s *Store) RevokeUntil(ctx context.Context, key string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	// Subtract before dividing so ceiling also works for the largest Duration.
	ms := (ttl - 1).Milliseconds() + 1
	if err := s.client.Eval(ctx, extendScript, []string{s.redisKey(key)}, ms).Err(); err != nil {
		return operationError{"extend revocation", err}
	}
	return nil
}

// Restore removes key from the store, re-allowing it.
func (s *Store) Restore(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.redisKey(key)).Err(); err != nil {
		return operationError{"restore", err}
	}
	return nil
}

// The server clock stays fixed during a script, so PTTL and SET compare the same instant.
const extendScript = `local ttl = redis.call('PTTL', KEYS[1])
if ttl ~= -1 and ttl < tonumber(ARGV[1]) then
 return redis.call('SET', KEYS[1], '1', 'PX', ARGV[1])
end
return 0`

// ROLE cannot run inside Redis scripts. Match a complete replication INFO field.
const strictReadScript = `local info = redis.pcall('INFO', 'replication')
if type(info) ~= 'string' or not string.find('\n' .. info .. '\n', '\nrole:master\r?\n') then
 return -1
end
return redis.call('EXISTS', KEYS[1])`
