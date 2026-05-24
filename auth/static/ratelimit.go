// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
)

// RateLimiter gates token-validation attempts. Implementations live outside
// this package — wire in data/limiters/tokenbucket, a Redis-backed limiter,
// or any other source through this consumer-side interface.
//
// The contract is failure-only: [RateLimiter.Allow] is a pure check that
// MUST NOT consume budget by itself, and [RateLimiter.RecordFailure] is
// the only path that debits the per-key counter. This split exists to
// defeat the success-resets-counter brute-force bypass: if successful
// validations could reset (or even just refund) the budget, an attacker
// holding a single valid token could interleave 1 valid + N invalid
// attempts and the invalid attempts would never accumulate.
//
// Implementations therefore typically:
//   - back Allow with a check-without-consume primitive (e.g. comparing
//     the failure counter for key against a configured limit);
//   - back RecordFailure with the actual increment that the limit
//     compares against, gated by a sliding window or TTL so the budget
//     naturally decays during quiet periods.
//
// A counter that never decays will eventually lock legitimate clients
// out of their own tokens; pick a self-decaying limiter (token-bucket
// refill, sliding window, TTL'd counter) rather than a monotonic
// integer.
type RateLimiter interface {
	// Allow reports whether a request bearing key may proceed. It MUST be
	// safe for concurrent use and MUST NOT debit the per-key budget — that
	// is the job of [RateLimiter.RecordFailure].
	Allow(ctx context.Context, key string) bool

	// RecordFailure debits the per-key failure budget. [RateLimitedStore]
	// calls it exactly once per failed validation; successes never call
	// RecordFailure. Implementations MUST be safe for concurrent use.
	RecordFailure(ctx context.Context, key string)
}

// KeyFunc derives the rate-limit key for a given context. Typical
// implementations return the client IP, an API-key prefix, or a tenant id.
//
// Returning an empty string causes [RateLimitedStore] to skip the rate-limit
// check for that request.
type KeyFunc func(ctx context.Context) string

// RateLimitedStore is a [TokenStore] decorator that consults a [RateLimiter]
// before delegating to the wrapped store. Only failed validations debit the
// limiter — successful validations never touch it. See the [RateLimiter]
// documentation for the rationale.
type RateLimitedStore struct {
	store   TokenStore
	limiter RateLimiter
	keyFn   KeyFunc
}

// NewRateLimitedStore wraps store with rate-limiting. Panics if store or
// limiter is nil — both are required and there is no safe default. When
// keyFn is nil, all requests share a single global rate-limit key.
func NewRateLimitedStore(store TokenStore, limiter RateLimiter, keyFn KeyFunc) *RateLimitedStore {
	if store == nil {
		panic("auth/static: RateLimitedStore requires a non-nil TokenStore")
	}
	if limiter == nil {
		panic("auth/static: RateLimitedStore requires a non-nil RateLimiter")
	}
	if keyFn == nil {
		keyFn = func(context.Context) string { return "global" }
	}
	return &RateLimitedStore{store: store, limiter: limiter, keyFn: keyFn}
}

// Validate implements [TokenStore].
//
// The flow is:
//  1. Resolve key. Empty key skips both Allow and RecordFailure entirely.
//  2. Call Allow. A false result is surfaced as [ErrRateLimited]; the key
//     is never embedded in the error text to keep client identifiers out
//     of logs and gRPC/HTTP error payloads.
//  3. Delegate to the wrapped store.
//  4. On failure (and only on failure) call RecordFailure so repeated bad
//     attempts eventually trip Allow.
func (s *RateLimitedStore) Validate(ctx context.Context, token string) (any, error) {
	key := s.keyFn(ctx)
	if key != "" && !s.limiter.Allow(ctx, key) {
		return nil, ErrRateLimited
	}

	data, err := s.store.Validate(ctx, token)
	if err != nil && key != "" {
		// Only failures consume budget. Successes intentionally do NOT
		// touch the limiter — any success-driven reset or refund would
		// let an attacker holding a single valid token interleave it
		// with bad attempts to keep the failure counter at zero
		// indefinitely.
		s.limiter.RecordFailure(ctx, key)
	}
	return data, err
}
