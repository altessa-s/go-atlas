// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

func BenchmarkRetry_Success(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		_, _ = natskvlease.Retry(ctx, func() (string, error) {
			return "ok", nil
		})
	}
}

func BenchmarkRetryWithConfig_Success(b *testing.B) {
	ctx := b.Context()
	cfg := natskvlease.RetryConfig{MaxRetries: 3, MaxElapsedTime: natskvlease.DefaultMaxElapsedTime}
	for b.Loop() {
		_, _ = natskvlease.RetryWithConfig(ctx, func() (int, error) {
			return 42, nil
		}, cfg)
	}
}
