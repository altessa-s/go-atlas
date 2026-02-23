// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"testing"
)

func BenchmarkParseTraceParent(b *testing.B) {
	header := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	b.ResetTimer()
	for b.Loop() {
		parseTraceParent(header)
	}
}

func BenchmarkTraceContext_Extract(b *testing.B) {
	tc := NewTraceContext()
	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		tc.Extract(ctx, carrier)
	}
}

func BenchmarkHeaderCarrier_Get(b *testing.B) {
	h := make(HeaderCarrier)
	h.Set("traceparent", "value")
	b.ResetTimer()
	for b.Loop() {
		h.Get("traceparent")
	}
}

func BenchmarkMapCarrier_Get(b *testing.B) {
	m := MapCarrier{"traceparent": "value"}
	b.ResetTimer()
	for b.Loop() {
		m.Get("traceparent")
	}
}
