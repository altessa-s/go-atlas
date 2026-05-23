// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

// ShouldRetryFunc decides whether [Do] should retry after err. Returning
// false causes [Do] to return err immediately without further attempts.
type ShouldRetryFunc func(err error) bool

// NextDelayFunc returns the delay before the next attempt. It receives
// the zero-based attempt number and the error from the most recent call.
// A non-positive result stops retrying and [Do] returns the last error.
// Use [Exponential] to build a standard exponential-backoff delay.
type NextDelayFunc func(attempt int, err error) time.Duration

// OnRetryFunc is an optional callback invoked after each failed attempt,
// before the delay sleep. It receives the attempt number, the error, and
// the computed delay that will be applied.
type OnRetryFunc func(attempt int, err error, nextDelay time.Duration)

// options holds the retry policy used by [Do]. Configure it through the
// generated [Option] values (see the With* constructors below).
//
// Attempt numbering starts at 0: maxAttempts=0 means "try once" (no
// retries), maxAttempts=3 means attempts 0..3 (4 total calls to fn),
// maxAttempts=-1 disables the attempt cap entirely.
type options struct {
	// maxAttempts is the maximum attempt index (inclusive). A value of 0
	// means the function is called exactly once; -1 disables the cap.
	// The field is deliberately unconstrained so that the zero value is a
	// legal configuration (single-shot with no retries).
	maxAttempts int `opt:"-"`

	// maxElapsedTime caps the total wall-clock time spent across all
	// attempts. A zero or negative value disables the limit.
	maxElapsedTime time.Duration `optval:"nonpositive"`

	// shouldRetry, when non-nil, filters retryable errors. A nil value
	// retries every error that reaches the retry loop.
	shouldRetry ShouldRetryFunc //

	// nextDelay computes the backoff before each retry. A nil value or a
	// non-positive return stops retrying.
	nextDelay NextDelayFunc //

	// onRetry is an optional callback invoked after each failed attempt.
	onRetry OnRetryFunc //
}

// WithMaxAttempts sets the maximum attempt index (inclusive). A value of
// 0 means fn is called exactly once, a positive N means up to N retries
// (N+1 total calls), and -1 disables the cap. The zero value of
// [options] already corresponds to "single attempt", so callers only
// need this option when they want retries.
func WithMaxAttempts(n int) Option {
	return func(o *options) {
		o.maxAttempts = n
	}
}
