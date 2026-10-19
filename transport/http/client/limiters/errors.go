// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"errors"
	"fmt"
	"time"
)

// ErrRateLimited indicates a request was blocked by a [RequestsLimiter]
// before it reached the network. Every denial of [RoundTripper] — and thus of
// the limiter installed by client.WithLimiter — is returned as a
// [RateLimitError] wrapping the limiter's error, including a wait that ended
// with the request context. Use [IsRateLimitError] to extract details.
var ErrRateLimited = errors.New("request rate limited")

// RateLimitError reports that a request was denied by a [RequestsLimiter].
// It matches [ErrRateLimited] through [errors.Is] and exposes the limiter's
// own error via [errors.Unwrap], so a limiter sentinel or a context error
// stays visible to [errors.Is].
type RateLimitError struct {
	// Host is the hostname that hit the rate limit.
	Host string
	// RetryAfter, when non-zero, suggests how long to wait before retrying.
	RetryAfter time.Duration
	// Err is the error returned by the limiter, or nil.
	Err error
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	msg := fmt.Sprintf("rate limit exceeded for host %q", e.Host)
	if e.RetryAfter > 0 {
		msg += fmt.Sprintf(", retry after %v", e.RetryAfter)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Is allows errors.Is to match [ErrRateLimited].
func (e *RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}

// Unwrap returns the limiter's error.
func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// IsRateLimitError unwraps err looking for a [RateLimitError].
// Returns nil when no match is found.
func IsRateLimitError(err error) *RateLimitError {
	target, _ := errors.AsType[*RateLimitError](err)
	return target
}
