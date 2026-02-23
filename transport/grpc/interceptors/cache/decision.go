// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Decision represents a cache decision containing whether to cache a response and for how long.
// The decision is made after the gRPC handler completes and includes both the caching
// determination and the time-to-live duration for cached entries.
type Decision struct {
	// ShouldCache indicates whether the response should be cached
	ShouldCache bool
	// TTL is the time-to-live for the cached response
	TTL time.Duration
}

// DecisionFunc determines whether a gRPC response should be cached and for how long.
// The function receives the request context, method name, request data, response data,
// and any error from the handler, then returns a Decision indicating caching behavior.
//
// Decision functions enable flexible caching policies based on response content,
// error types, request characteristics, or any other application-specific criteria.
type DecisionFunc func(ctx context.Context, method string, req, resp any, err error) Decision

// DefaultSuccessOnlyDecision creates a decision function that caches only successful responses.
// Responses are cached when err is nil and resp is non-nil, using the specified TTL.
// All error responses are not cached to prevent caching transient failures.
//
// This is the most common caching policy for typical applications where only
// successful operations should be cached.
func DefaultSuccessOnlyDecision(ttl time.Duration) DecisionFunc {
	return func(ctx context.Context, method string, req, resp any, err error) Decision {
		if err == nil && resp != nil {
			return Decision{
				ShouldCache: true,
				TTL:         ttl,
			}
		}
		return Decision{ShouldCache: false}
	}
}

// DefaultSuccessAndErrorDecision creates a decision function that caches both successful responses
// and specific types of errors with different TTL values.
// Successful responses (no error, non-nil response) are cached with successTTL.
// Specific error codes (NotFound, AlreadyExists) are cached with errorTTL to prevent
// repeated expensive operations for predictable failures.
//
// This policy is useful for applications where certain errors are expensive to compute
// and unlikely to change quickly (e.g., resource not found, validation errors).
func DefaultSuccessAndErrorDecision(successTTL, errorTTL time.Duration) DecisionFunc {
	return func(ctx context.Context, method string, req, resp any, err error) Decision {
		if err == nil && resp != nil {
			return Decision{ShouldCache: true, TTL: successTTL}
		}

		if err != nil {
			if st, ok := status.FromError(err); ok {
				// Only cache certain error codes
				switch st.Code() {
				case codes.NotFound, codes.AlreadyExists:
					return Decision{ShouldCache: true, TTL: errorTTL}
				default:
					// Don't cache other error types
					break
				}
			}
		}

		return Decision{ShouldCache: false}
	}
}
