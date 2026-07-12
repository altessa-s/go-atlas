// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context_test

import (
	"context"
	"testing"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

func BenchmarkApplyTimeout(b *testing.B) {
	ctx := b.Context() // no deadline => timer context is created

	b.ReportAllocs()
	for b.Loop() {
		tctx, cancel := corecontext.ApplyTimeout(ctx, time.Second)
		cancel()
		_ = tctx
	}
}

func BenchmarkApplyTimeout_ExistingDeadline(b *testing.B) {
	ctx, cancel := context.WithTimeout(b.Context(), time.Hour)
	defer cancel()

	b.ReportAllocs()
	for b.Loop() {
		tctx, tcancel := corecontext.ApplyTimeout(ctx, time.Second)
		tcancel()
		_ = tctx
	}
}

func BenchmarkWithDefault(b *testing.B) {
	ctx := b.Context() // no deadline => timer context is created

	b.ReportAllocs()
	for b.Loop() {
		tctx, cancel := corecontext.WithDefault(ctx, time.Second)
		cancel()
		_ = tctx
	}
}
