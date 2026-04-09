// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

func BenchmarkHandleNoPanic(b *testing.B) {
	ctx := context.Background()
	for b.Loop() {
		func() {
			defer panics.Handle(ctx)
			// Normal code path — no panic.
		}()
	}
}

func BenchmarkHandleWithPanic(b *testing.B) {
	ctx := context.Background()
	for b.Loop() {
		func() {
			defer panics.Handle(ctx)
			panic("boom")
		}()
	}
}

func BenchmarkMustNonNilHit(b *testing.B) {
	v := "value"
	for b.Loop() {
		panics.MustNonNil(v, "v is required")
	}
}

func BenchmarkMustResultHit(b *testing.B) {
	op := func() (int, error) { return 42, nil }
	var sink int
	for b.Loop() {
		sink = panics.MustResult(op())
	}
	_ = sink
}

func BenchmarkMustErrorNil(b *testing.B) {
	for b.Loop() {
		panics.MustError(nil)
	}
}
