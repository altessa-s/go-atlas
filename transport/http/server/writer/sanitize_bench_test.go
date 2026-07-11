// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"testing"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// BenchmarkSanitizeResponse_NoInputOnly measures the common case: a proto
// response with no populated INPUT_ONLY field. The strict detection pass finds
// nothing, so no clone is allocated.
func BenchmarkSanitizeResponse_NoInputOnly(b *testing.B) {
	msg := &testpb.Resource{Name: "public", Description: "d"}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := sanitizeResponse(msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSanitizeResponse_StripsInputOnly measures the leak-path case: the
// detection pass finds a populated INPUT_ONLY field, so the message is cloned
// and stripped.
func BenchmarkSanitizeResponse_StripsInputOnly(b *testing.B) {
	msg := &testpb.Resource{
		Name:     "public",
		Password: "secret",
		Profile:  &testpb.Profile{DisplayName: "dn", Secret: "s"},
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := sanitizeResponse(msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSanitizeResponse_NonProto measures the non-proto fast path.
func BenchmarkSanitizeResponse_NonProto(b *testing.B) {
	data := map[string]any{"name": "n"}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := sanitizeResponse(data); err != nil {
			b.Fatal(err)
		}
	}
}
