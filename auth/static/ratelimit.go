// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"fmt"
)

// RateLimiter gates token-validation attempts. Implementations live outside
// this package — wire in data/limiters/tokenbucket, a Redis-backed limiter,
// or any other source through this consumer-side interface.
type RateLimiter interface {
	// Allow reports whether a request bearing key may proceed. Implementations
	// must be safe for concurrent use.
	Allow(ctx context.Context, key string) bool

	// Reset clears any accumulated state for key. Called by [RateLimitedStore]
	// after a successful validation so legitimate clients are not held back
	// by past failures.
	Reset(key string)
}

// KeyFunc derives the rate-limit key for a given context. Typical
// implementations return the client IP, an API-key prefix, or a tenant id.
//
// Returning an empty string causes [RateLimitedStore] to skip the rate-limit
// check for that request.
type KeyFunc func(ctx context.Context) string

// RateLimitedStore is a [TokenStore] decorator that consults a [RateLimiter]
// before delegating to the wrapped store. On a successful validation the
// rate-limiter is reset for the request key so future legitimate traffic is
// not punished for prior failures.
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
func (s *RateLimitedStore) Validate(ctx context.Context, token string) (any, error) {
	key := s.keyFn(ctx)
	if key != "" && !s.limiter.Allow(ctx, key) {
		return nil, fmt.Errorf("%w: key=%q", ErrRateLimited, key)
	}

	data, err := s.store.Validate(ctx, token)
	if err == nil && key != "" {
		s.limiter.Reset(key)
	}
	return data, err
}
