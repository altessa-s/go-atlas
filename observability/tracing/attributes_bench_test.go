// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func BenchmarkString(b *testing.B) {
	for b.Loop() {
		String("key", "value")
	}
}

func BenchmarkAttributesPool(b *testing.B) {
	for b.Loop() {
		attrs := GetAttributes()
		*attrs = append(*attrs, String("k", "v"))
		PutAttributes(attrs)
	}
}

func BenchmarkCloneAttributes(b *testing.B) {
	attrs := []Attribute{
		String("a", "1"), Int("b", 2), Float64("c", 3.14),
		Bool("d", true), String("e", "5"),
	}
	b.ResetTimer()
	for b.Loop() {
		CloneAttributes(attrs)
	}
}

func BenchmarkFilterAttributesByKey(b *testing.B) {
	attrs := []Attribute{
		String("http.method", "GET"),
		String("http.path", "/api"),
		String("db.system", "postgres"),
		String("http.status", "200"),
		Int("http.duration", 100),
	}
	b.ResetTimer()
	for b.Loop() {
		for range FilterAttributesByKey(attrs, "http.") {
		}
	}
}
