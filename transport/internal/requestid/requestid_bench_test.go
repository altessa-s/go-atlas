// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"
)

func BenchmarkGenerator_Extract_Existing(b *testing.B) {
	gen := NewGenerator()
	headers := &benchHeaderGetter{headers: map[string]string{
		DefaultHTTPHeaderName: "550e8400-e29b-41d4-a716-446655440000",
	}}

	for b.Loop() {
		gen.Extract(headers)
	}
}

func BenchmarkGenerator_Extract_Generate(b *testing.B) {
	gen := NewGenerator()

	for b.Loop() {
		gen.Extract(nil)
	}
}

func BenchmarkNewContext(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		NewContext(ctx, "test-id")
	}
}

func BenchmarkFromContext(b *testing.B) {
	ctx := NewContext(b.Context(), "test-id")
	for b.Loop() {
		FromContext(ctx)
	}
}

type benchHeaderGetter struct {
	headers map[string]string
}

func (m *benchHeaderGetter) GetHeader(name string) string {
	return m.headers[name]
}
