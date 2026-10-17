// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"

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

// BenchmarkExtractTraceContext measures server-side trace extraction with a
// realistic amount of unrelated incoming metadata.
func BenchmarkExtractTraceContext(b *testing.B) {
	pairs := []string{"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
	for i := range 20 {
		pairs = append(pairs, "x-meta-"+strconv.Itoa(i), "value")
	}
	ctx := metadata.NewIncomingContext(b.Context(), metadata.Pairs(pairs...))
	i := ServerInterceptor(tracing.Noop())

	b.ReportAllocs()
	for b.Loop() {
		_ = i.extractTraceContext(ctx)
	}
}
