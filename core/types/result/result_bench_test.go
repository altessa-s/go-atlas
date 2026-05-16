// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package result_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/core/types/result"
)

var errBench = errors.New("result_bench: sentinel")

func BenchmarkOk(b *testing.B) {
	var sink result.Result[int]
	for b.Loop() {
		sink = result.Ok(42)
	}
	_ = sink
}

func BenchmarkErr(b *testing.B) {
	var sink result.Result[int]
	for b.Loop() {
		sink = result.Err[int](errBench)
	}
	_ = sink
}

func BenchmarkOfOk(b *testing.B) {
	var sink result.Result[int]
	for b.Loop() {
		sink = result.Of(42, error(nil))
	}
	_ = sink
}

func BenchmarkOfErr(b *testing.B) {
	var sink result.Result[int]
	for b.Loop() {
		sink = result.Of(0, errBench)
	}
	_ = sink
}

func BenchmarkGet(b *testing.B) {
	r := result.Ok(42)
	var v int
	var err error
	for b.Loop() {
		v, err = r.Get()
	}
	_ = v
	_ = err
}

func BenchmarkOrDefaultOk(b *testing.B) {
	r := result.Ok(42)
	var sink int
	for b.Loop() {
		sink = r.OrDefault(99)
	}
	_ = sink
}

func BenchmarkOrDefaultErr(b *testing.B) {
	r := result.Err[int](errBench)
	var sink int
	for b.Loop() {
		sink = r.OrDefault(99)
	}
	_ = sink
}

func BenchmarkOrElseOk(b *testing.B) {
	r := result.Ok(42)
	fn := func(error) int { return 7 }
	var sink int
	for b.Loop() {
		sink = r.OrElse(fn)
	}
	_ = sink
}

func BenchmarkOrElseErr(b *testing.B) {
	r := result.Err[int](errBench)
	fn := func(error) int { return 7 }
	var sink int
	for b.Loop() {
		sink = r.OrElse(fn)
	}
	_ = sink
}
