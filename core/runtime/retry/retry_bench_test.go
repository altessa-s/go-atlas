// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/retry"
)

func BenchmarkDo(b *testing.B) {
	ctx := b.Context()
	failErr := errors.New("fail")

	// Benchmark overhead with immediate success
	b.Run("Success", func(b *testing.B) {
		cfg := retry.Config{} // Default 0 attempts (just one try)
		for b.Loop() {
			_ = retry.Do(ctx, cfg, func(ctx context.Context) error {
				return nil
			})
		}
	})

	// Benchmark overhead with immediate retries
	b.Run("Retries", func(b *testing.B) {
		cfg := retry.Config{
			MaxAttempts: 10,
			NextDelay:   func(int, error) time.Duration { return 0 }, // No delay
		}
		for b.Loop() {
			_ = retry.Do(ctx, cfg, func(ctx context.Context) error {
				return failErr
			})
		}
	})

	b.Run("ExponentialCalc", func(b *testing.B) {
		exp := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Second})
		i := 0
		for b.Loop() {
			exp(i%10, nil)
			i++
		}
	})
}
