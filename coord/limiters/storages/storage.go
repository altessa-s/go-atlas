// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/coord/limiters"
)

// ErrLimitExceeded is returned by [Storage.Allow] when a request exceeds
// its rate limit. It shares its identity with [limiters.ErrLimitExceeded].
var ErrLimitExceeded = limiters.ErrLimitExceeded

// LimitInfo contains information about the current rate limit state.
type LimitInfo struct {
	Remaining int64

	// Reset is the Unix time, in whole seconds rounded down, at which the
	// oldest request in the sliding window leaves it and frees a slot. On an
	// admission into an empty window that is the admitted request itself.
	Reset int64
}

// Storage defines the backend for rate limiting.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Allow checks if a request should be allowed based on the rate limit.
	// It returns LimitInfo with current state and an error if limit is exceeded.
	// The key should uniquely identify the client (IP, token, user ID, etc.).
	// The limit is the maximum number of requests allowed.
	// The period is the time period for which the limit applies.
	Allow(ctx context.Context, key string, limit int64, period time.Duration) (*LimitInfo, error)

	// Reset resets the rate limit for a specific key.
	// This can be useful for manual overrides or administrative actions.
	Reset(ctx context.Context, key string) error
}

// PeriodValidator is an optional storage capability for validating configured
// windows at limiter construction. Storage must still validate dynamic periods
// passed directly to Allow.
type PeriodValidator interface {
	ValidatePeriod(time.Duration) error
}
