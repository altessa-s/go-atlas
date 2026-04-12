// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestHeaderCarrier(t *testing.T) {
	h := make(HeaderCarrier)
	h.Set("traceparent", "value")
	require.Equal(t, "value", h.Get("traceparent"))
	require.Len(t, h.Keys(), 1)
}

func TestHeaderCarrier_FromHTTP(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-abc-def-01")
	carrier := HeaderCarrier(h)
	require.Equal(t, "00-abc-def-01", carrier.Get("traceparent"))
}

func TestMapCarrier(t *testing.T) {
	m := MapCarrier{}
	m.Set("key", "value")
	require.Equal(t, "value", m.Get("key"))
	require.Empty(t, m.Get("missing"))
	require.Len(t, m.Keys(), 1)
}

func TestTraceContext_Fields(t *testing.T) {
	tc := NewTraceContext()
	fields := tc.Fields()
	require.True(t, slices.Contains(fields, "traceparent"), "Fields should contain traceparent")
	require.True(t, slices.Contains(fields, "tracestate"), "Fields should contain tracestate")
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
			require.Equal(t, tt.wantOK, ok)
			if ok {
				require.Equal(t, tt.sampled, sc.traceFlags&0x01 == 0x01)
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
	require.NotNil(t, remote, "expected remote span context")
	require.True(t, remote.IsValid(), "expected valid")
	require.True(t, remote.IsSampled(), "expected sampled")
	require.True(t, remote.IsRemote(), "expected remote")
	require.Equal(t, "0af7651916cd43dd8448eb211c80319c", remote.TraceID())
	require.Equal(t, "b7ad6b7169203331", remote.SpanID())
}

func TestTraceContext_Extract_Empty(t *testing.T) {
	tc := NewTraceContext()
	ctx := tc.Extract(t.Context(), MapCarrier{})
	require.Nil(t, RemoteSpanContextFromContext(ctx), "expected nil remote span context for empty carrier")
}

func TestTraceContext_Extract_WithTracestate(t *testing.T) {
	tc := NewTraceContext()
	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		"tracestate":  "congo=t61rcWkgMzE",
	}
	ctx := tc.Extract(t.Context(), carrier)
	remote := RemoteSpanContextFromContext(ctx)
	require.NotNil(t, remote, "expected remote span context")
	require.Equal(t, "congo=t61rcWkgMzE", remote.TraceState())
}

func TestCompositePropagator(t *testing.T) {
	tc := NewTraceContext()
	comp := NewCompositePropagator(tc)

	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := comp.Extract(t.Context(), carrier)
	require.NotNil(t, RemoteSpanContextFromContext(ctx), "expected remote span context from composite")
	require.NotEmpty(t, comp.Fields(), "Fields should not be empty")
}

func TestRemoteSpanContext_TraceFlags(t *testing.T) {
	tc := NewTraceContext()
	carrier := MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := tc.Extract(t.Context(), carrier)
	remote := RemoteSpanContextFromContext(ctx)
	require.Equal(t, tracing.TraceFlags(0x01), remote.TraceFlags())
}
