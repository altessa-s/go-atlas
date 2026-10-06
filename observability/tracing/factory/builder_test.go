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

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/tracing/factory"
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

			cfg := config.DefaultTracing()
			cfg.Enabled = true
			cfg.Type = config.TracingTypeOTLP
			cfg.Sampler = &config.TracingSampler{Type: config.SamplerTypeAlwaysOn}
			cfg.Adapters = &config.TracingAdapters{OTLP: &config.TracingOTLP{
				Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
				Protocol:    config.OTLPProtocolHTTP,
				Insecure:    true,
				Compression: tc.compression,
			}}

			tracer, err := factory.New(&cfg).Build(t.Context())
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
