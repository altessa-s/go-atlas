// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

func BenchmarkIsNilPlainNil(b *testing.B) {
	var sink bool
	for b.Loop() {
		sink = nilcheck.IsNil(nil)
	}
	_ = sink
}

func BenchmarkIsNilNilPointer(b *testing.B) {
	var p *string
	var sink bool
	for b.Loop() {
		sink = nilcheck.IsNil(p)
	}
	_ = sink
}

func BenchmarkIsNilNonNilPointer(b *testing.B) {
	s := "value"
	p := &s
	var sink bool
	for b.Loop() {
		sink = nilcheck.IsNil(p)
	}
	_ = sink
}

func BenchmarkIsNilString(b *testing.B) {
	var sink bool
	for b.Loop() {
		sink = nilcheck.IsNil("hello")
	}
	_ = sink
}

func BenchmarkRequireNotNilHit(b *testing.B) {
	s := "value"
	var err error
	for b.Loop() {
		err = nilcheck.RequireNotNil(&s, "field")
	}
	_ = err
}

func BenchmarkRequireNotNilMiss(b *testing.B) {
	var err error
	for b.Loop() {
		err = nilcheck.RequireNotNil(nil, "field")
	}
	_ = err
}

func BenchmarkCheckerChain(b *testing.B) {
	v1 := "a"
	v2 := 1
	var err error
	for b.Loop() {
		err = nilcheck.NewChecker("Bench").
			Check(&v1, "s").
			Check(&v2, "i").
			Error()
	}
	_ = err
}
