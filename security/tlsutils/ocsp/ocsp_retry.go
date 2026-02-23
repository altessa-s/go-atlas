// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"context"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
)

// RetryPolicy defines the interface for OCSP retry policies.
// Implement this interface to provide custom retry behavior for OCSP requests.
type RetryPolicy interface {
	// NextRetry returns the duration to wait before the next retry.
	// The attempt parameter is the current attempt number starting from 0.
	// Returns 0 to stop retrying.
	NextRetry(attempt int) time.Duration

	// MaxAttempts returns the maximum number of retry attempts.
	// Returns -1 for unlimited retries.
	MaxAttempts() int

	// ShouldRetry determines if the error is retryable.
	ShouldRetry(err error) bool
}

// RetryConfig holds retry configuration for OCSP operations.
// Use with ExecuteWithRetry to add retry behavior to OCSP requests.
type RetryConfig struct {
	// Policy is the retry policy to use.
	Policy RetryPolicy

	// Context for cancellation.
	Context context.Context

	// OnRetry is an optional callback invoked before each retry attempt.
	OnRetry func(attempt int, err error, nextDelay time.Duration)
}

// ExecuteWithRetry executes a function with retry logic.
// If config or config.Policy is nil, the function is executed once without retry.
// Returns the last error encountered or nil on success.
// If config.Context is nil, context.Background() is used.
//
// Example:
//
//	err := ExecuteWithRetry(func() error {
//		return doSomething()
//	}, &RetryConfig{Policy: myPolicy, Context: ctx})
//
//nolint:contextcheck // This function intentionally accepts nil ctx and falls back to Background for library ergonomics.
func ExecuteWithRetry(fn func() error, config *RetryConfig) error {
	if config == nil || config.Policy == nil {
		return fn()
	}

	ctx := config.Context
	ctx = corecontext.OrBackground(ctx)

	return coreretry.Do(ctx, coreretry.Config{
		MaxAttempts: config.Policy.MaxAttempts(),
		ShouldRetry: config.Policy.ShouldRetry,
		NextDelay: func(attempt int, err error) time.Duration {
			return config.Policy.NextRetry(attempt)
		},
		OnRetry: config.OnRetry,
	}, func(context.Context) error {
		return fn()
	})
}
