// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import "testing"

func BenchmarkNewRegistry(b *testing.B) {
	for b.Loop() {
		NewRegistry()
	}
}

func BenchmarkRegistry_GetEncoder(b *testing.B) {
	r := NewRegistry()
	r.RegisterEncoder("bench/test", &mockCodec{mime: "bench/test"})
	for b.Loop() {
		r.GetEncoder("bench/test")
	}
}

func BenchmarkRegistry_GetDecoder(b *testing.B) {
	r := NewRegistry()
	r.RegisterDecoder("bench/test", &mockCodec{mime: "bench/test"})
	for b.Loop() {
		r.GetDecoder("bench/test")
	}
}

func BenchmarkRegistry_Negotiate(b *testing.B) {
	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})
	r.RegisterEncoder("application/xml", &mockCodec{mime: "application/xml"})

	for b.Loop() {
		r.Negotiate("application/json, application/xml;q=0.9")
	}
}
