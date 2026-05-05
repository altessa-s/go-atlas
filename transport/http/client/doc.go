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
//   - Proxy: declarative outbound proxy configuration via [WithProxy], [WithProxyURL], [WithProxyFunc],
//     or [WithoutProxy]; defaults to [http.ProxyFromEnvironment].
//   - Observability: request logging through [WithLogger], structured error types for inspection,
//     and optional integration with [observability/health] via [WithHealthCoordinator] (see Health below).
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
// # Health integration
//
// Pass an [observability/health.Coordinator] via [WithHealthCoordinator] to
// register the client as a health checker. The aggregate status is derived
// from the global and per-host circuit breakers and a sliding window of
// retry rate (see [DefaultHealthRetryWindow], [DefaultHealthRetryDegradedThreshold],
// [DefaultHealthRetryMinSamples]):
//
//   - all breakers closed and retry rate ≤ threshold → SERVING
//   - any half-open or open-while-others-closed, or retry rate above threshold → DEGRADED
//   - all breakers open → NOT_SERVING
//
// Subscribers via [Coordinator.Subscribe] receive immediate updates on every
// breaker state transition; pull-based callers see the cached value through
// [Coordinator.CheckStatus]. Per-host services for hosts configured via
// [WithCircuitBreakerSettings] can be opted into with [WithPerHostHealthChecks].
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
