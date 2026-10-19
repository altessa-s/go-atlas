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
//   - Resilience: built-in retry policies with exponential backoff and jitter ([WithRetryMax], [WithRetryWait]);
//     [WithoutRetry] sends each request once and returns every reply unclassified. Retries after a 429/503
//     reply wait at least its Retry-After ([WithRetryAfterMax], [ParseRetryAfter]).
//   - Protection: a circuit breaker per destination host, so one failing endpoint never rejects requests to
//     another; tune hosts with [WithCircuitBreakerSettings] and bound lazily created breakers with [WithMaxHostBreakers].
//   - Rate Limiting: pluggable client-side rate limiting via [WithLimiter] and [limiters.RequestsLimiter].
//   - SSRF Protection: default blocking of connections to private/local IPs; see [WithSSRFProtection] and [WithSSRFAllowedCIDRs].
//     [SSRFDialControl] exports the dial guard for services that build their own transport.
//   - Proxy: declarative outbound proxy configuration via [WithProxy], [WithProxyURL], [WithProxyFunc],
//     or [WithoutProxy]; environment proxies apply only with [WithoutSSRFProtection].
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
//   - limiters.IsRateLimitError extracts limiters.RateLimitError (matches
//     limiters.ErrRateLimited), returned for a [WithLimiter] denial.
//   - [IsRetryExhaustedError] extracts [RetryExhaustedError] (matches [ErrMaxRetriesExceeded]).
//   - [IsSSRFError] extracts [SSRFError] (matches [ErrSSRFBlocked]).
//   - [IsTemporaryError] checks for transient conditions that may succeed on retry.
//
// # Health integration
//
// Pass an [observability/health.Coordinator] via [WithHealthCoordinator] to
// register the client as a health checker. The aggregate status is derived
// from the per-host circuit breakers (plus the shared overflow breaker once used) and a sliding window of
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
//
// SSRF protection also blocks loopback and disables environment proxies. An
// explicit proxy option is allowed: the dial to the proxy is guarded (allow its
// network with [WithSSRFAllowedCIDRs]), the proxy resolves the target host, and
// a target given as a literal private IP is refused before reaching the proxy.
// A proxy TLS configuration, custom TLS dialers, a proxy resolver set on a
// caller transport and non-standard transports require [WithoutSSRFProtection];
// otherwise construction panics with [ErrSSRFUnsafeTransport].
package client
