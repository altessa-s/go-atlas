// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"path/filepath"
	"testing"
)

func BenchmarkStatic_ID(b *testing.B) {
	p := NewStatic("bench-id-12345")
	b.ResetTimer()
	for b.Loop() {
		_ = p.ID()
	}
}

func BenchmarkFile_ID(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "service.id")
	p, err := NewFile(path)
	if err != nil {
		b.Fatal(err)
	}
	// Trigger initialization
	_ = p.ID()

	b.ResetTimer()
	for b.Loop() {
		_ = p.ID()
	}
}

func BenchmarkNewStatic(b *testing.B) {
	for b.Loop() {
		_ = NewStatic("some-id")
	}
}

func BenchmarkNewWithProvider(b *testing.B) {
	p := NewStatic("bench-id")
	b.ResetTimer()
	for b.Loop() {
		_, _ = NewWithProvider(p)
	}
}
