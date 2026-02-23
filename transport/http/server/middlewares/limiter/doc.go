// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package limiter provides middleware for per-request rate limiting.
//
// The middleware delegates rate decisions to a [limiters.Limiter] backend
// (e.g., token bucket). It enriches the limiter context with the client IP
// (from the realip middleware) and Bearer token (from the Authorization
// header) when available.
//
// Standard rate-limit response headers (X-RateLimit-Limit,
// X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After) are set
// automatically. On limit exceeded, the middleware returns 429 Too Many
// Requests.
//
// The middleware declares a dependency on "realip" for ordering.
//
// # Example
//
//	mw := limiter.New(rateLimiter,
//	    limiter.WithIgnorePaths("/health"),
//	)
//	handler := mw.Handler(yourHandler)
package limiter
