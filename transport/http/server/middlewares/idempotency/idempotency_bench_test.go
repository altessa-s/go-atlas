// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"io"
	"log/slog"
	"testing"
)

func BenchmarkDefaultKeyValidator(b *testing.B) {
	key := "550e8400-e29b-41d4-a716-446655440000"
	for b.Loop() {
		DefaultKeyValidator(key) //nolint:errcheck
	}
}

func BenchmarkBuildKey(b *testing.B) {
	m := &middleware{}
	for b.Loop() {
		m.buildKey("POST", "/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	}
}

func BenchmarkKeyLogAttrs(b *testing.B) {
	const key = "550e8400-e29b-41d4-a716-446655440000"

	debug := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	info := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))

	for _, mode := range []KeyLogMode{KeyLogHashed, KeyLogFull, KeyLogOff} {
		b.Run(mode.String(), func(b *testing.B) {
			m := New(nil, WithLogger(debug), WithKeyLogMode(mode))
			ctx := b.Context()
			for b.Loop() {
				m.keyLogAttrs(ctx, "POST", key)
			}
		})
	}

	// The production shape: debug disabled, so nothing is built at all.
	b.Run("debug_disabled", func(b *testing.B) {
		m := New(nil, WithLogger(info))
		ctx := b.Context()
		for b.Loop() {
			m.keyLogAttrs(ctx, "POST", key)
		}
	})
}
