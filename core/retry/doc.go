// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package retry provides small, stdlib-only retry helpers.
//
// It is intended for retry loops that need:
//   - context cancellation
//   - deterministic delay policies
//   - no `time.After` allocations in loops (timer reuse)
//
// All exported functions are safe for concurrent use.
//
// # Usage
//
//	err := retry.Do(ctx, func(ctx context.Context) error {
//	    return doThing(ctx)
//	},
//	    retry.WithMaxAttempts(3), // attempts 0..3 (4 total)
//	    retry.WithShouldRetry(func(err error) bool { return true }),
//	    retry.WithNextDelay(retry.Exponential(retry.ExponentialConfig{
//	        BaseDelay: 500 * time.Millisecond,
//	    })),
//	)
//
// # Performance
//
// The retry loop reuses a single time.Timer across attempts to avoid per-attempt allocations.
package retry
