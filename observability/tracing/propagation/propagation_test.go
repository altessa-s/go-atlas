// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"net/http"
	"slices"
	"testing"
)

func TestHeaderCarrier(t *testing.T) {
	h := make(HeaderCarrier)
	h.Set("traceparent", "value")
	if got := h.Get("traceparent"); got != "value" {
		t.Errorf("Get = %q, want %q", got, "value")
	}
	keys := h.Keys()
	if len(keys) != 1 {
		t.Errorf("Keys len = %d", len(keys))
	}
}

func TestHeaderCarrier_FromHTTP(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-abc-def-01")
	carrier := HeaderCarrier(h)
	if got := carrier.Get("traceparent"); got != "00-abc-def-01" {
		t.Errorf("Get = %q", got)
	}
}

func TestMapCarrier(t *testing.T) {
	m := MapCarrier{}
	m.Set("key", "value")
	if got := m.Get("key"); got != "value" {
		t.Errorf("Get = %q", got)
	}
	if got := m.Get("missing"); got != "" {
		t.Errorf("Get missing = %q", got)
	}
	keys := m.Keys()
	if len(keys) != 1 {
		t.Errorf("Keys len = %d", len(keys))
	}
}

func TestTraceContext_Fields(t *testing.T) {
	tc := NewTraceContext()
	fields := tc.Fields()
	if !slices.Contains(fields, "traceparent") {
		t.Error("Fields should contain traceparent")
	}
	if !slices.Contains(fields, "tracestate") {
		t.Error("Fields should contain tracestate")
	}
}

func TestParseTraceParent(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantOK  bool
		sampled bool
	}{
		{
			"valid sampled",
			"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			true, true,
		},
		{
			"valid not sampled",
			"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00",
			true, false,
		},
		{
			"with spaces",
			"  00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01  ",
			true, true,
		},
		{
			"uppercase",
			"00-0AF7651916CD43DD8448EB211C80319C-B7AD6B7169203331-01",
			true, true,
		},
		{"empty", "", false, false},
		{"invalid format", "not-a-traceparent", false, false},
		{"invalid version ff", "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", false, false},
		{"all-zero trace ID", "00-00000000000000000000000000000000-b7ad6b7169203331-01", false, false},
		{"all-zero span ID", "00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, ok := parseTraceParent(tt.header)
			if ok != tt.wantOK {
				t.Errorf("parseTraceParent() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && tt.sampled != (sc.traceFlags&0x01 == 0x01) {
				t.Errorf("sampled = %v, want %v", sc.traceFlags&0x01 == 0x01, tt.sampled)
			}
		})
	}
}

func TestTraceContext_ExtractInject_RoundTrip(t *testing.T) {
	tc := NewTraceContext()
	carrier := MapCarrier{}
	carrier.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	ctx := tc.Extract(t.Context(), carrier)
	remote := RemoteSpanContextFromContext(ctx)
	if remote == nil {
		t.Fatal("expected remote span context")
	}
	if !remote.IsValid() {
		t.Error("expected valid")
	}
	if !remote.IsSampled() {
		t.Error("expected sampled")
	}
	if !remote.IsRemote() {
		t.Error("expected remote")
	}
	if remote.TraceID() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("TraceID = %q", remote.TraceID())
	}
	if remote.SpanID() != "b7ad6b7169203331" {
		t.Errorf("SpanID = %q", remote.SpanID())
	}
}

func TestTraceContext_Extract_Empty(t *testing.T) {
	tc := NewTraceContext()
	ctx := tc.Extract(t.Context(), MapCarrier{})
	if RemoteSpanContextFromContext(ctx) != nil {
		t.Error("expected nil remote span context for empty carrier")
	}
}

func TestTraceContext_Extract_WithTracestate(t *testing.T) {
	tc := NewTraceContext()
	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		"tracestate":  "congo=t61rcWkgMzE",
	}
	ctx := tc.Extract(t.Context(), carrier)
	remote := RemoteSpanContextFromContext(ctx)
	if remote == nil {
		t.Fatal("expected remote span context")
	}
	if remote.TraceState() != "congo=t61rcWkgMzE" {
		t.Errorf("TraceState = %q", remote.TraceState())
	}
}

func TestCompositePropagator(t *testing.T) {
	tc := NewTraceContext()
	comp := NewCompositePropagator(tc)

	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := comp.Extract(t.Context(), carrier)
	if RemoteSpanContextFromContext(ctx) == nil {
		t.Error("expected remote span context from composite")
	}

	fields := comp.Fields()
	if len(fields) == 0 {
		t.Error("Fields should not be empty")
	}
}

func TestRemoteSpanContext_TraceFlags(t *testing.T) {
	tc := NewTraceContext()
	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := tc.Extract(t.Context(), carrier)
	remote := RemoteSpanContextFromContext(ctx)
	if remote.TraceFlags() != 0x01 {
		t.Errorf("TraceFlags = %v", remote.TraceFlags())
	}
}
