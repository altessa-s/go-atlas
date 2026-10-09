// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package singleflight_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/singleflight"
)

func BenchmarkDo(b *testing.B) {
	g := singleflight.New[string, int]()
	ctx := b.Context()
	fn := func(context.Context) (int, error) { return 1, nil }
	b.ReportAllocs()
	for b.Loop() {
		_, _ = g.Do(ctx, "key", fn)
	}
}
