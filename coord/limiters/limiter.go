// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"errors"
)

// ErrLimitExceeded is returned when the rate limit has been exceeded.
var ErrLimitExceeded = errors.New("rate limit exceeded")

// LimitInfo contains information about the rate limit status.
// It provides details about the current limit, remaining requests,
// and when the limit will reset.
type LimitInfo struct {
	// Limit is the maximum number of requests allowed in the current window.
	Limit int64

	// Remaining is the number of requests remaining in the current window.
	Remaining int64

	// Reset is the Unix timestamp when the rate limit window resets.
	Reset int64
}

// IsLimitExceeded returns true if the rate limit has been exceeded.
func (l *LimitInfo) IsLimitExceeded() bool {
	return l.Remaining <= 0
}

// Limiter defines the interface for rate limiters used in interceptors and middlewares.
type Limiter interface {
	// Limit checks if the request should be rate limited.
	// Returns LimitInfo with current limit status.
	// Returns ErrLimitExceeded if the limit is exceeded.
	// If returned error is a gRPC status.Status (for gRPC) or has an HTTP status code,
	// it will be returned directly to the client.
	Limit(ctx context.Context) (*LimitInfo, error)
}

// Func is an adapter to allow the use of ordinary functions as Limiters.
// This enables functional programming patterns where a simple function
// can be used in place of a full Limiter implementation.
type Func func(ctx context.Context) (*LimitInfo, error)

// Limit calls the underlying function.
func (f Func) Limit(ctx context.Context) (*LimitInfo, error) {
	return f(ctx)
}
