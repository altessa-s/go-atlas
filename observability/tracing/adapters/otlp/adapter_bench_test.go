// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

// BenchmarkConvertToResourceSpans measures the OTLP encode step in isolation:
// SpanData -> protobuf ResourceSpans, without any network export.
func BenchmarkConvertToResourceSpans(b *testing.B) {
	a := &Adapter{resource: createResource(defaultOptions())}
	end := time.Now()
	spans := []adapters.SpanData{{
		TraceID:   adapters.TraceID{0x01, 0x02, 0x03, 0x04},
		SpanID:    adapters.SpanID{0x0a, 0x0b},
		Name:      "benchmark-span",
		Kind:      adapters.SpanKindServer,
		StartTime: end.Add(-time.Millisecond),
		EndTime:   end,
		Attributes: []adapters.Attribute{
			{Key: "http.method", Value: "GET"},
			{Key: "http.status_code", Value: 200},
		},
		Status: adapters.StatusOK,
	}}

	b.ReportAllocs()
	for b.Loop() {
		_ = a.convertToResourceSpans(spans)
	}
}
