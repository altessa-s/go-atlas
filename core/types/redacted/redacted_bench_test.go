// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

func BenchmarkString(b *testing.B) {
	s := redacted.RedactedString("super-secret")
	var sink string
	for b.Loop() {
		sink = s.String()
	}
	_ = sink
}

func BenchmarkGoString(b *testing.B) {
	s := redacted.RedactedString("super-secret")
	var sink string
	for b.Loop() {
		sink = s.GoString()
	}
	_ = sink
}

func BenchmarkLogValue(b *testing.B) {
	s := redacted.RedactedString("super-secret")
	var sink slog.Value
	for b.Loop() {
		sink = s.LogValue()
	}
	_ = sink
}

func BenchmarkExpose(b *testing.B) {
	s := redacted.RedactedString("super-secret")
	var sink string
	for b.Loop() {
		sink = s.Expose()
	}
	_ = sink
}

func BenchmarkIsEmpty(b *testing.B) {
	s := redacted.RedactedString("super-secret")
	var sink bool
	for b.Loop() {
		sink = s.IsEmpty()
	}
	_ = sink
}
