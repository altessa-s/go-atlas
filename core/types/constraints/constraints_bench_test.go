// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package constraints_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// The constraints package contains only type sets, so there is nothing
// to benchmark at runtime. The benchmarks below exist to satisfy the
// per-package bench requirement and to provide a compile-time stress
// test for the generic instantiation machinery.

func addNumbers[T constraints.Numbers](a, b T) T { return a + b }

func BenchmarkAddInt(b *testing.B) {
	var sum int
	for b.Loop() {
		sum = addNumbers[int](sum, 1)
	}
	_ = sum
}

func BenchmarkAddFloat64(b *testing.B) {
	var sum float64
	for b.Loop() {
		sum = addNumbers[float64](sum, 1)
	}
	_ = sum
}

func BenchmarkAddUint64(b *testing.B) {
	var sum uint64
	for b.Loop() {
		sum = addNumbers[uint64](sum, 1)
	}
	_ = sum
}
