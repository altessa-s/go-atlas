// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

func BenchmarkSome(b *testing.B) {
	var sink optional.Optional[int]
	for b.Loop() {
		sink = optional.Some(42)
	}
	_ = sink
}

func BenchmarkNone(b *testing.B) {
	var sink optional.Optional[int]
	for b.Loop() {
		sink = optional.None[int]()
	}
	_ = sink
}

func BenchmarkOfSome(b *testing.B) {
	var sink optional.Optional[int]
	for b.Loop() {
		sink = optional.Of(42, true)
	}
	_ = sink
}

func BenchmarkOfNone(b *testing.B) {
	var sink optional.Optional[int]
	for b.Loop() {
		sink = optional.Of(42, false)
	}
	_ = sink
}

func BenchmarkGet(b *testing.B) {
	o := optional.Some(42)
	var v int
	var ok bool
	for b.Loop() {
		v, ok = o.Get()
	}
	_ = v
	_ = ok
}

func BenchmarkOrDefaultSome(b *testing.B) {
	o := optional.Some(42)
	var sink int
	for b.Loop() {
		sink = o.OrDefault(99)
	}
	_ = sink
}

func BenchmarkOrDefaultNone(b *testing.B) {
	o := optional.None[int]()
	var sink int
	for b.Loop() {
		sink = o.OrDefault(99)
	}
	_ = sink
}

func BenchmarkOrElseSome(b *testing.B) {
	o := optional.Some(42)
	fn := func() int { return 7 }
	var sink int
	for b.Loop() {
		sink = o.OrElse(fn)
	}
	_ = sink
}

func BenchmarkOrElseNone(b *testing.B) {
	o := optional.None[int]()
	fn := func() int { return 7 }
	var sink int
	for b.Loop() {
		sink = o.OrElse(fn)
	}
	_ = sink
}
