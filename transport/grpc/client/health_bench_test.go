// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
)

// BenchmarkClientCheckHealth measures the synchronous read path used by
// the coordinator's pull cycle.
func BenchmarkClientCheckHealth(b *testing.B) {
	coord := health.New()
	b.Cleanup(coord.Close)

	c, err := New(b.Context(), unreachableTarget, WithInsecure(), WithHealthCoordinator(coord))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close(b.Context()) })

	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = c.health.CheckHealth(ctx)
	}
}
