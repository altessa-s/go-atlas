// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"
)

func TestParseMethod(t *testing.T) {
	tests := []struct {
		name        string
		fullMethod  string
		wantService string
		wantMethod  string
	}{
		{"standard", "/mypackage.MyService/MyMethod", "mypackage.MyService", "MyMethod"},
		{"no_leading_slash", "mypackage.MyService/MyMethod", "", "mypackage.MyService/MyMethod"},
		{"empty", "", "", ""},
		{"only_slash", "/", "", ""},
		{"no_method_slash", "/ServiceOnly", "", "ServiceOnly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method := parseMethod(tt.fullMethod)
			require.Equal(t, tt.wantService, svc)
			require.Equal(t, tt.wantMethod, method)
		})
	}
}

func TestSpanAttributes(t *testing.T) {
	attrs := spanAttributes("/pkg.Svc/Method")
	require.Len(t, attrs, 3)
}

func TestMetadataCarrier(t *testing.T) {
	md := metadata.New(nil)
	carrier := metadataCarrier(md)

	carrier.Set("traceparent", "00-abc-def-01")
	got := carrier.Get("traceparent")
	require.Equal(t, "00-abc-def-01", got)

	got = carrier.Get("nonexistent")
	require.Equal(t, "", got)

	keys := carrier.Keys()
	require.Len(t, keys, 1)
}

func TestDefaultSpanName(t *testing.T) {
	got := defaultSpanName("/pkg.Svc/Method")
	require.Equal(t, "/pkg.Svc/Method", got)
}

func TestServerInterceptor_NilTracer(t *testing.T) {
	i := ServerInterceptor(nil)
	require.NotNil(t, i, "should not be nil")
	require.Equal(t, "tracing", i.Name())
}

func TestClientInterceptor_NilTracer(t *testing.T) {
	i := ClientInterceptor(nil)
	require.NotNil(t, i, "should not be nil")
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := ServerInterceptor(nil)
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}
