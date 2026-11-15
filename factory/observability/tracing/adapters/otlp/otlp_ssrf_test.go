// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/tracing/adapters/otlp"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	tracingfactory "github.com/altessa-s/go-atlas/factory/observability/tracing"
)

// The OTLP/HTTP exporter keeps SSRF protection on and allows only the
// configured collector: a collector on loopback is reached, but a redirect it
// returns to another loopback port is refused before the connection is made.
func TestBuild_OTLPHTTPRedirectToOtherLoopbackPortIsBlocked(t *testing.T) {
	t.Parallel()
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	var collectorHits atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorHits.Add(1)
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(collector.Close)

	cfg := observabilityconfig.DefaultTracing()
	cfg.Enabled = true
	cfg.Type = observabilityconfig.TracingTypeOTLP
	cfg.Sampler = &observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn}
	cfg.Adapters = &observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{
		Endpoint: strings.TrimPrefix(collector.URL, "http://"),
		Protocol: observabilityconfig.OTLPProtocolHTTP,
		Insecure: true,
	}}

	tracer, err := tracingfactory.New(&cfg).UseAdapter(observabilityconfig.TracingTypeOTLP, factory.Builder()).Build(t.Context())
	require.NoError(t, err)
	_, span := tracer.Recorder("test").Start(t.Context(), "op")
	span.End()
	_ = tracer.Shutdown(t.Context()) // the export fails: the redirect is refused

	require.Positive(t, collectorHits.Load(), "the configured loopback collector must be reached")
	require.Zero(t, targetHits.Load(), "a redirect to another loopback port must be blocked")
}
