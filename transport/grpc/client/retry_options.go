// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import "time"

// retryOptions controls the gRPC service-config-level retry policy injected
// into every connection when [WithRetry] is set. The policy uses exponential
// backoff and limits retries to statusCodes. All fields are populated by
// generated [RetryOption] setters in retry_options_gen.go.
type retryOptions struct {
	// maxAttempts is the maximum number of attempts, including the initial call.
	maxAttempts int `optgen:"default=DefaultMaxRetryAttempts" optval:"positive"`

	// initialBackoff is the initial backoff duration between retries.
	initialBackoff time.Duration `optgen:"default=DefaultInitialBackoff"`

	// maxBackoff is the maximum backoff duration.
	maxBackoff time.Duration `optgen:"default=DefaultMaxBackoff"`

	// backoffMultiplier is the multiplier for exponential backoff.
	backoffMultiplier float64 `optgen:"default=DefaultBackoffMultiplier" optval:"positive"`

	// statusCodes are the gRPC status code names that trigger a retry.
	statusCodes []string `optgen:"default=DefaultRetryableStatusCodes"`
}
