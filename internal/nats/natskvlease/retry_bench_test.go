// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
)

func BenchmarkRetry_Success(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		_, _ = natskvlease.Retry(ctx, func() (string, error) {
			return "ok", nil
		})
	}
}

func BenchmarkRetry_WithOptions_Success(b *testing.B) {
	ctx := b.Context()
	opts := []natskvlease.RetryOption{natskvlease.WithRetryMaxAttempts(3), natskvlease.WithRetryJitter(0)}
	for b.Loop() {
		_, _ = natskvlease.Retry(ctx, func() (int, error) {
			return 42, nil
		}, opts...)
	}
}
