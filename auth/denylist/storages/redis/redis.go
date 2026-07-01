// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"iter"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// revokedValue is the sentinel stored at a revocation key; only its presence
// matters.
const revokedValue = "1"

// scanBatch is the COUNT hint passed to SCAN. It bounds how many keys Redis
// examines per round trip during a stream; it is a hint, not a hard page size.
const scanBatch = 256

// Store is a Redis-backed distributed exact revocation store. Each revoked key
// is a single Redis string keyed as keyPrefix+key with value "1"; Redis expires
// bounded revocations natively via per-key TTL.
//
// Store satisfies the negcache Authoritative interface structurally (via
// [Store.IsRevoked]) and implements [probfilter.DataLoader], so it can be both
// the authoritative tier behind a negative cache and the source that rebuilds
// the cache's filter. It is safe for concurrent use.
type Store struct {
	client redis.UniversalClient
	opts   *options
}

var _ probfilter.DataLoader = (*Store)(nil)

// New returns a Store backed by client. The client is required; pass
// [WithKeyPrefix] to override the default key namespace.
func New(client redis.UniversalClient, opts ...Option) *Store {
	return &Store{client: client, opts: newOptions(opts...)}
}

// redisKey returns the namespaced Redis key for a bare revocation key.
func (s *Store) redisKey(key string) string {
	return s.opts.keyPrefix + key
}

// IsRevoked reports, authoritatively, whether key is revoked. A bounded
// revocation whose TTL has elapsed is gone from Redis and reports false.
func (s *Store) IsRevoked(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, s.redisKey(key)).Result()
	if err != nil {
		return false, coreerrs.Wrapf(err, "check revocation for key %q", key)
	}
	return n > 0, nil
}

// Revoke denies key permanently, until a matching [Store.Restore]. The key is
// written with no expiry.
func (s *Store) Revoke(ctx context.Context, key string) error {
	if err := s.client.Set(ctx, s.redisKey(key), revokedValue, 0).Err(); err != nil {
		return coreerrs.Wrapf(err, "revoke key %q", key)
	}
	return nil
}

// RevokeUntil denies key for ttl, after which Redis expires it. A non-positive
// ttl is a no-op — the token is already invalid on its own — mirroring the
// in-memory denylist's non-positive expiry.
func (s *Store) RevokeUntil(ctx context.Context, key string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	if err := s.client.Set(ctx, s.redisKey(key), revokedValue, ttl).Err(); err != nil {
		return coreerrs.Wrapf(err, "revoke key %q until ttl", key)
	}
	return nil
}

// Restore removes key from the store, re-allowing it.
func (s *Store) Restore(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.redisKey(key)).Err(); err != nil {
		return coreerrs.Wrapf(err, "restore key %q", key)
	}
	return nil
}

// StreamValues implements [probfilter.DataLoader]. It SCANs every revocation key
// under the store's prefix, strips the prefix, and yields each bare key. The
// iterator stops early when ctx is canceled or the consumer stops pulling; on a
// Redis or context error it yields a single ("", err) pair and returns.
func (s *Store) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		match := s.opts.keyPrefix + "*"
		var cursor uint64
		for {
			if err := ctx.Err(); err != nil {
				yield("", coreerrs.Wrap(err, "stream revoked keys"))
				return
			}
			keys, next, err := s.client.Scan(ctx, cursor, match, scanBatch).Result()
			if err != nil {
				yield("", coreerrs.Wrap(err, "scan revoked keys"))
				return
			}
			for _, k := range keys {
				if !yield(strings.TrimPrefix(k, s.opts.keyPrefix), nil) {
					return
				}
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	}
}

// Count implements [probfilter.DataLoader]. It returns -1 (unknown): an exact
// count would require a full SCAN of the keyspace, which is as expensive as the
// rebuild itself, so callers size the filter from -1 (unknown) instead.
func (s *Store) Count(_ context.Context) (int64, error) {
	return -1, nil
}
