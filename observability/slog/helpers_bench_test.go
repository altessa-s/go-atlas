// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"errors"
	"log/slog"
	"testing"
)

func BenchmarkError(b *testing.B) {
	err := errors.New("test error")
	b.ResetTimer()
	for b.Loop() {
		Error(err)
	}
}

func BenchmarkString(b *testing.B) {
	for b.Loop() {
		String("key", "value")
	}
}

func BenchmarkMaskingReplaceAttr(b *testing.B) {
	fn := MaskingReplaceAttr([]string{"password", "token", "secret"}, "***")
	attr := slog.String("password", "supersecret")
	b.ResetTimer()
	for b.Loop() {
		fn(nil, attr)
	}
}

func BenchmarkModule(b *testing.B) {
	for b.Loop() {
		Module("http-server")
	}
}
