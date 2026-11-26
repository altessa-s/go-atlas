// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tokenbucket implements rule-based rate limiting using a sliding window algorithm.
// It provides IP/CIDR-based rate limiting, client-specific limits, and authentication token support
// with configurable storage backends and sophisticated rule matching that prioritizes specific rules.
//
// The RuleLimiter is thread-safe and can be used concurrently from multiple goroutines.
// It automatically handles rule matching, IP caching with LRU algorithm, and storage backend operations.
//
// # Features
//
//   - Rule-based limiting: support for default, CIDR, and specific client rules.
//   - Unlimited token and IP rules bypass storage.
//   - Pluggable storage: memory (single-instance) or Redis/NATS (distributed).
//   - Advanced matching: prioritizes more specific rules over broader ones.
//   - Performance: uses LRU cache for IP classification to minimize storage hits.
//   - Storage keys: IPv6 clients share a bucket per [WithIPv6KeyPrefixBits]
//     prefix (default /64); tokens are keyed by their SHA-256 digest, never
//     stored raw.
//
// # Usage
//
//	storage := memory.New()
//	defer storage.Close()
//	limiter, err := tokenbucket.New(
//	    tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
//	    storage,
//	    tokenbucket.WithRules(&tokenbucket.RateLimitRule{
//	        Target:            "192.168.1.0/24",
//	        RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 1000, Period: time.Minute},
//	    }),
//	    tokenbucket.WithExtractClientIPAddress(limiters.ClientIPFromContext),
//	)
//	if err != nil {
//	    return err
//	}
//
//	if _, err := limiter.Limit(limiters.ContextWithClientIP(ctx, "192.168.1.5")); errors.Is(err, limiters.ErrLimitExceeded) {
//	    // rate limited
//	}
package tokenbucket
