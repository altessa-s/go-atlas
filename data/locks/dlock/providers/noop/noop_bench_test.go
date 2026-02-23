// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/noop"
)

func BenchmarkProvider_Lock(b *testing.B) {
	p := noop.New()
	ctx := b.Context()
	for b.Loop() {
		lk, _ := p.Lock(ctx, "bench-key")
		_ = lk.Release(ctx)
	}
}

func BenchmarkProvider_GetLockInfo(b *testing.B) {
	p := noop.New()
	ctx := b.Context()
	for b.Loop() {
		_, _ = p.GetLockInfo(ctx, "bench-key")
	}
}
