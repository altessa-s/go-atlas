// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/factory/observability/tracing/adapters/otlp"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	tracingfactory "github.com/altessa-s/go-atlas/factory/observability/tracing"
)

// The OTLP adapter must honor the configured protocol and compression: an
// http exporter posts to /v1/traces, gzip-encoded only when compression is on.
func TestBuild_OTLPProtocolAndCompression(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		compression bool
		wantGzip    bool
	}{
		{"http_without_compression", false, false},
		{"http_with_compression", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			type request struct{ path, encoding string }
			got := make(chan request, 8)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got <- request{r.URL.Path, r.Header.Get("Content-Encoding")}
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)

			cfg := observabilityconfig.DefaultTracing()
			cfg.Enabled = true
			cfg.Type = observabilityconfig.TracingTypeOTLP
			cfg.Sampler = &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn}
			cfg.Adapters = &observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{
				Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
				Protocol:    observabilityconfig.OTLPProtocolHTTP,
				Insecure:    true,
				Compression: tc.compression,
			}}

			tracer, err := tracingfactory.New(&cfg).UseAdapter(observabilityconfig.TracingTypeOTLP, factory.Builder()).Build(t.Context())
			require.NoError(t, err)
			_, span := tracer.Recorder("test").Start(t.Context(), "op")
			span.End()
			require.NoError(t, tracer.Shutdown(t.Context()))

			var req request
			select {
			case req = <-got:
			case <-time.After(10 * time.Second):
				t.Fatal("no OTLP/HTTP export received")
			}
			require.Equal(t, "/v1/traces", req.path, "the http protocol must be used")
			if tc.wantGzip {
				require.Equal(t, "gzip", req.encoding)
			} else {
				require.Empty(t, req.encoding, "compression: false must not gzip the export")
			}
		})
	}
}

// Both OTLP exporters share the configured proxy: the HTTP exporter sends
// its request to the proxy (the collector host does not resolve, so only
// the proxy can see it), the gRPC exporter tunnels through it with CONNECT.
func TestBuild_OTLPProxyAppliesToBothProtocols(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		protocol   observabilityconfig.OTLPProtocol
		endpoint   string
		wantMethod string
	}{
		{"http", observabilityconfig.OTLPProtocolHTTP, "collector.invalid:4318", http.MethodPost},
		{"grpc", observabilityconfig.OTLPProtocolGRPC, "127.0.0.1:4317", http.MethodConnect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			type request struct{ method, host string }
			got := make(chan request, 8)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got <- request{r.Method, r.Host}
				w.WriteHeader(http.StatusForbidden)
			}))
			t.Cleanup(proxy.Close)

			cfg := observabilityconfig.DefaultTracing()
			cfg.Enabled = true
			cfg.Type = observabilityconfig.TracingTypeOTLP
			cfg.Sampler = &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn}
			cfg.Adapters = &observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{
				Endpoint: tc.endpoint,
				Protocol: tc.protocol,
				Insecure: true,
				Proxy:    &proxyconfig.Config{Mode: proxyconfig.ModeURL, URL: redacted.URL(proxy.URL)},
			}}

			tracer, err := tracingfactory.New(&cfg).UseAdapter(observabilityconfig.TracingTypeOTLP, factory.Builder()).Build(t.Context())
			require.NoError(t, err)
			_, span := tracer.Recorder("test").Start(t.Context(), "op")
			span.End()
			_ = tracer.Shutdown(t.Context())

			var req request
			select {
			case req = <-got:
			case <-time.After(10 * time.Second):
				t.Fatal("the OTLP export did not go through the configured proxy")
			}
			require.Equal(t, tc.wantMethod, req.method)
			require.Equal(t, tc.endpoint, req.host)
		})
	}
}
