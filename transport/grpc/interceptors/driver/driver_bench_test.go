// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import "testing"

var (
	benchResp any
	benchErr  error
)

// BenchmarkNoopDriver_PreCall measures the per-request passthrough cost of the
// no-op driver's pre-call hook.
func BenchmarkNoopDriver_PreCall(b *testing.B) {
	d := NoopDriver()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = d.PreCall(ctx, nil)
	}
}

// BenchmarkNoopDriver_PostCall measures the per-request passthrough cost of
// the no-op driver's post-call hook.
func BenchmarkNoopDriver_PostCall(b *testing.B) {
	d := NoopDriver()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchErr = d.PostCall(ctx, nil, nil)
	}
}
