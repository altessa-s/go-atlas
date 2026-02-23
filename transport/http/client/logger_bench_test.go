// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"log/slog"
	"testing"
)

func BenchmarkLogger_Info(b *testing.B) {
	l := NewLogger(slog.New(slog.DiscardHandler))
	for b.Loop() {
		l.Info("test message", "key", "value")
	}
}

func BenchmarkLogger_Error(b *testing.B) {
	l := NewLogger(slog.New(slog.DiscardHandler))
	for b.Loop() {
		l.Error("test error", "key", "value")
	}
}

func BenchmarkNewLogger(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	for b.Loop() {
		NewLogger(logger)
	}
}
