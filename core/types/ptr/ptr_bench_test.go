// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ptr_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/ptr"
)

func BenchmarkWrapInt(b *testing.B) {
	var sink *int
	for b.Loop() {
		sink = ptr.Wrap(42)
	}
	_ = sink
}

func BenchmarkWrapString(b *testing.B) {
	var sink *string
	for b.Loop() {
		sink = ptr.Wrap("hello")
	}
	_ = sink
}

func BenchmarkWrapNonZeroHit(b *testing.B) {
	var sink *int
	for b.Loop() {
		sink = ptr.WrapNonZero(42)
	}
	_ = sink
}

func BenchmarkWrapNonZeroMiss(b *testing.B) {
	var sink *int
	for b.Loop() {
		sink = ptr.WrapNonZero(0)
	}
	_ = sink
}

func BenchmarkUnwrapHit(b *testing.B) {
	p := ptr.Wrap(42)
	var sink int
	for b.Loop() {
		sink = ptr.Unwrap(p)
	}
	_ = sink
}

func BenchmarkUnwrapNilDefault(b *testing.B) {
	var sink int
	for b.Loop() {
		sink = ptr.Unwrap[int](nil, 99)
	}
	_ = sink
}
