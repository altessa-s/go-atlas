// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package static provides transport-neutral static-token authentication.
//
// The package exposes a small [TokenStore] interface and a default
// [InMemoryStore] implementation suitable for API keys, opaque tokens, and
// other pre-shared credentials. It is used by the gRPC and HTTP auth
// adapters under transport/* to plug static-token authentication into
// interceptors and middlewares.
//
// # Storage
//
// [InMemoryStore] keys tokens by their HMAC-SHA256 digest using a
// per-instance random key. Lookups are a single map probe — there is no
// linear scan or early-break loop, so timing depends on the input length
// only and not on token position or membership. Plaintext tokens are
// never stored in the map; only their digests are kept in memory.
//
// # Usage
//
//	store := static.NewInMemoryStore(
//	    static.WithInitialTokens(map[string]any{
//	        "sk_live_xxx": UserInfo{ID: "user1", Role: "admin"},
//	        "sk_test_xxx": UserInfo{ID: "user2", Role: "viewer"},
//	    }),
//	)
//	data, err := store.Validate(ctx, token)
//
// # Rate limiting
//
// [RateLimitedStore] is a decorator that consults a caller-supplied
// [RateLimiter] before delegating to the wrapped store. The package does
// not ship a rate-limiter implementation — wire one from
// data/limiters/tokenbucket or another source through the
// [RateLimiter] interface.
//
// # Metrics
//
// Pass a [*Metrics] via [WithMetrics] to record validation outcomes,
// active-token count, and validation latency.
package static
