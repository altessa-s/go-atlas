// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

func BenchmarkParseMethod(b *testing.B) {
	for b.Loop() {
		parseMethod("/mypackage.MyService/MyMethod")
	}
}

func BenchmarkSpanAttributes(b *testing.B) {
	for b.Loop() {
		spanAttributes("/mypackage.MyService/MyMethod")
	}
}

func BenchmarkMetadataCarrier_Get(b *testing.B) {
	md := metadata.Pairs("traceparent", "00-abc-def-01")
	carrier := metadataCarrier(md)
	for b.Loop() {
		carrier.Get("traceparent")
	}
}
