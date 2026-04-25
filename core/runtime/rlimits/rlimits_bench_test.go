// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rlimits_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/rlimits"
)

// Apply mutates process-wide resource limits and cannot be safely
// benchmarked in-loop. The benchmarks below cover the option
// construction path, which is what callers touch from their startup
// code.

func BenchmarkWithDisableCoreDumps(b *testing.B) {
	var sink rlimits.Option
	for b.Loop() {
		sink = rlimits.WithDisableCoreDumps()
	}
	_ = sink
}

func BenchmarkWithMaxOpenFiles(b *testing.B) {
	var sink rlimits.Option
	for b.Loop() {
		sink = rlimits.WithMaxOpenFiles(1024)
	}
	_ = sink
}

func BenchmarkWithMemoryBytes(b *testing.B) {
	var sink rlimits.Option
	for b.Loop() {
		sink = rlimits.WithMemoryBytes(1 << 30)
	}
	_ = sink
}
