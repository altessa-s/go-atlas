// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func BenchmarkNewSpanContextImpl_NoParent(b *testing.B) {
	for b.Loop() {
		newSpanContextImpl(nil)
	}
}

func BenchmarkNewSpanContextImpl_WithParent(b *testing.B) {
	parent := newSpanContextImpl(nil)
	b.ResetTimer()
	for b.Loop() {
		newSpanContextImpl(parent)
	}
}

func BenchmarkSpanContextImpl_TraceID(b *testing.B) {
	sc := newSpanContextImpl(nil)
	b.ResetTimer()
	for b.Loop() {
		sc.TraceID()
	}
}

func BenchmarkSpanContextImpl_SpanID(b *testing.B) {
	sc := newSpanContextImpl(nil)
	b.ResetTimer()
	for b.Loop() {
		sc.SpanID()
	}
}
