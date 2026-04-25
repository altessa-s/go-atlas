// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package limiter provides gRPC interceptors for rate limiting incoming requests.
// It supports flexible rate limiting strategies, graceful degradation, and automatic
// client notifications through response headers.
//
// Use [ServerInterceptor] (or the standalone [ServerUnaryInterceptor] /
// [ServerStreamInterceptor]) to apply rate limiting. The interceptor injects
// the standard rate-limit response headers ([RateLimitMetadataKey],
// [RateLimitRemaining], [RateLimitReset]) on every response so that clients
// can implement adaptive backoff.
//
// When the limiter returns [sharedlimiter.ErrLimitExceeded], the interceptor
// responds with [codes.ResourceExhausted]. Other limiter errors are handled
// according to the configured fallback behavior (allow, deny, or error).
//
// Example:
//
//	interceptor := limiter.ServerInterceptor(
//	    rateLimiter,
//	    limiter.WithFallbackBehavior(fallback.Deny),
//	)
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//	    grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
//	)
package limiter
