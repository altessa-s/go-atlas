// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
)

// RequestsLimiter is the interface for client-side request rate limiting.
// Implementations must be safe for concurrent use. The key supplied to
// [RequestsLimiter.Allow] is typically the target hostname, but can be any
// string used to partition rate limits.
type RequestsLimiter interface {
	// Allow checks if a request for the given key is allowed under the current rate limit.
	// The key is typically a hostname, but can be any identifier for grouping requests.
	// Returns an error if the request should be denied due to rate limiting.
	//
	// Example implementation might limit requests to 100 per minute per hostname:
	//
	//	err := limiter.Allow(ctx, "api.example.com")
	//	if err != nil {
	//	    // Request denied due to rate limit
	//	}
	Allow(ctx context.Context, key string) error
}
