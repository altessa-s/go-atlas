// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package client provides a resilient HTTP client with automatic retries, circuit breaker protection,
// rate limiting, and SSRF safeguards. It wraps the standard [net/http.Client] with additional
// features for high-availability systems.
//
// Two constructors are available:
//
//   - [New] returns a bare *[net/http.Client] suitable for use with any code that
//     expects the standard type.
//   - [NewHTTPClient] returns an [HTTPClient] with convenience methods (Get, PostJSON,
//     fluent [RequestBuilder], etc.) layered on top.
//
// # Features
//
//   - Connection Management: automatic TCP connection pooling and keep-alive configuration.
//   - Resilience: built-in retry policies with exponential backoff and jitter ([WithRetryMax], [WithRetryWait]).
//   - Protection: per-host circuit breakers ([WithCircuitBreakerSettings]) and bulkhead patterns to prevent cascading failures.
//   - Rate Limiting: pluggable client-side rate limiting via [WithLimiter] and [limiters.RequestsLimiter].
//   - SSRF Protection: optional blocking of connections to private/local IPs; see [WithSSRFProtection] and [WithSSRFAllowedCIDRs].
//   - Observability: request logging through [WithLogger] and structured error types for inspection.
//
// # Error Handling
//
// All errors implement [errors.Is] matching against package-level sentinels.
// Use the typed extraction helpers for detailed inspection:
//
//   - [IsUnexpectedStatusError] extracts [UnexpectedStatusError] (matches [ErrUnexpectedStatus]).
//   - [IsCircuitBreakerError] / [IsCircuitBreakerOpen] inspect [CircuitBreakerError] (matches [ErrCircuitBreakerOpen]).
//   - [IsResponseSizeError] extracts [ResponseSizeError] (matches [ErrResponseSizeExceeded]).
//   - [IsRateLimitError] extracts [RateLimitError] (matches [ErrRateLimited]).
//   - [IsRetryExhaustedError] extracts [RetryExhaustedError] (matches [ErrMaxRetriesExceeded]).
//   - [IsSSRFError] extracts [SSRFError] (matches [ErrSSRFBlocked]).
//   - [IsTemporaryError] checks for transient conditions that may succeed on retry.
//
// # Usage
//
//	c := client.New(
//	    client.WithRetryMax(3),
//	    client.WithRetryWait(1*time.Second, 5*time.Second),
//	)
//
//	resp, _ := c.Get("https://api.example.com/v1/data")
//	defer resp.Body.Close()
package client
