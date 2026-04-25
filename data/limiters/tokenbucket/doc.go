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
//   - Pluggable storage: memory (single-instance) or Redis/NATS (distributed).
//   - Advanced matching: prioritizes more specific rules over broader ones.
//   - Performance: uses LRU cache for IP classification to minimize storage hits.
//
// # Usage
//
//	storage := memory.New()
//	config := &RateLimitConfig{
//	    Default: RateLimitSettings{Limit: 100, Period: time.Minute},
//	    Rules: []RateLimitRule{
//	        {CIDR: "192.168.1.0/24", Settings: RateLimitSettings{Limit: 1000}},
//	    },
//	}
//	limiter := New(config, storage)
//	defer storage.Close()
//
//	if allowed, _ := limiter.Allow(ctx, "192.168.1.5", ""); !allowed {
//	    // rate limited
//	}
package tokenbucket
