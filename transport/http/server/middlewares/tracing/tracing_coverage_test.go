// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"

	obstracing "github.com/altessa-s/go-atlas/observability/tracing"
)

// recordedSpan captures what the middleware wrote onto a span.
type recordedSpan struct {
	name       string
	kind       obstracing.SpanKind
	attrs      map[string]any
	status     obstracing.StatusCode
	statusDesc string
	ended      bool
}

type fakeTracer struct {
	mu    sync.Mutex
	spans []*recordedSpan
}

func (f *fakeTracer) Recorder(string, ...obstracing.RecorderOption) obstracing.Recorder { return f }
func (f *fakeTracer) WithScope(string) obstracing.Tracer                                { return f }

func (f *fakeTracer) Start(ctx context.Context, name string, opts ...obstracing.SpanStartOption) (context.Context, obstracing.Span) {
	cfg := obstracing.ApplySpanStartOptions(opts...)
	rs := &recordedSpan{name: name, kind: cfg.Kind(), attrs: map[string]any{}}
	for _, a := range cfg.Attributes() {
		rs.attrs[a.Key] = a.Value
	}
	f.mu.Lock()
	f.spans = append(f.spans, rs)
	f.mu.Unlock()
	return ctx, &fakeSpan{rec: rs, mu: &f.mu}
}

func (f *fakeTracer) recorded() []recordedSpan {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedSpan, 0, len(f.spans))
	for _, s := range f.spans {
		out = append(out, *s)
	}
	return out
}

type fakeSpan struct {
	obstracing.Span
	rec *recordedSpan
	mu  *sync.Mutex
}

func (s *fakeSpan) End(...obstracing.SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.ended = true
}

func (s *fakeSpan) SetStatus(code obstracing.StatusCode, desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.status, s.rec.statusDesc = code, desc
}

func (s *fakeSpan) SetAttributes(attrs ...obstracing.Attribute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range attrs {
		s.rec.attrs[a.Key] = a.Value
	}
}

type ctxKey struct{}

// markingPropagator records extracted headers in the context.
type markingPropagator struct{}

func (markingPropagator) Inject(context.Context, propagation.TextMapCarrier) {}
func (markingPropagator) Fields() []string                                   { return []string{"x-trace"} }
func (markingPropagator) Extract(ctx context.Context, c propagation.TextMapCarrier) context.Context {
	return context.WithValue(ctx, ctxKey{}, c.Get("x-trace"))
}

func statusHandler(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "tracing", tracing.Name())
}

func TestHandler_RecordsServerSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       int
		wantStatus obstracing.StatusCode
		wantDesc   string
	}{
		{name: "ok", code: http.StatusOK, wantStatus: obstracing.StatusOK},
		{name: "client error", code: http.StatusNotFound, wantStatus: obstracing.StatusError, wantDesc: "Not Found"},
		{name: "server error", code: http.StatusBadGateway, wantStatus: obstracing.StatusError, wantDesc: "Bad Gateway"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &fakeTracer{}
			h := tracing.New(tr).Handler(statusHandler(tc.code))

			req := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/users?token=secret", nil)
			req.Header.Set("User-Agent", "probe/1.0")
			req.RemoteAddr = "198.51.100.4:5555"
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tc.code, rec.Code)

			spans := tr.recorded()
			require.Len(t, spans, 1)
			s := spans[0]
			require.Equal(t, "GET /v1/users", s.name)
			require.Equal(t, obstracing.SpanKindServer, s.kind)
			require.True(t, s.ended)
			require.Equal(t, tc.wantStatus, s.status)
			require.Equal(t, tc.wantDesc, s.statusDesc)
			require.Equal(t, tc.code, s.attrs[tracing.HTTPStatusCodeKey])
			require.Equal(t, "https", s.attrs[tracing.HTTPSchemeKey])
			require.Equal(t, "https://api.example.com/v1/users", s.attrs[tracing.HTTPURLKey], "query string is never exported")
			require.Equal(t, "probe/1.0", s.attrs[tracing.HTTPUserAgentKey])
			require.Equal(t, "198.51.100.4", s.attrs[tracing.NetPeerIPKey])
		})
	}
}

func TestHandler_ClientIPAndPlainScheme(t *testing.T) {
	t.Parallel()

	tr := &fakeTracer{}
	h := tracing.New(tr).Handler(statusHandler(http.StatusOK))

	req := httptest.NewRequest(http.MethodPost, "http://svc.local/orders", nil)
	req.TLS = nil
	req = req.WithContext(clientip.NewContext(req.Context(), netip.MustParseAddr("203.0.113.9")))
	h.ServeHTTP(httptest.NewRecorder(), req)

	s := tr.recorded()[0]
	require.Equal(t, "http", s.attrs[tracing.HTTPSchemeKey])
	require.Equal(t, "203.0.113.9", s.attrs[tracing.NetPeerIPKey], "real IP from realip wins over RemoteAddr")

	req = httptest.NewRequest(http.MethodGet, "http://svc.local/x", nil)
	req.RemoteAddr = "not-a-host-port"
	req.TLS = &tls.ConnectionState{}
	h.ServeHTTP(httptest.NewRecorder(), req)
	s = tr.recorded()[1]
	require.Equal(t, "not-a-host-port", s.attrs[tracing.NetPeerIPKey])
	require.Equal(t, "https", s.attrs[tracing.HTTPSchemeKey])
}

func TestHandler_Options(t *testing.T) {
	t.Parallel()

	tr := &fakeTracer{}
	var extracted any
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extracted = r.Context().Value(ctxKey{})
		w.WriteHeader(http.StatusNoContent)
	})
	h := tracing.WrapHandler(tr, next,
		tracing.WithIgnorePaths("/healthz"),
		tracing.WithIgnorePatterns(regexp.MustCompile(`^/metrics`)),
		tracing.WithLogger(slog.New(slog.DiscardHandler)),
		tracing.WithPropagator(markingPropagator{}),
		tracing.WithSpanNameFunc(func(r *http.Request) string { return "custom:" + r.URL.Path }),
	)

	for _, path := range []string{"/healthz", "/metrics/x"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNoContent, rec.Code)
	}
	require.Empty(t, tr.recorded(), "ignored paths are not traced")

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("x-trace", "parent-123")
	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := tr.recorded()
	require.Len(t, spans, 1)
	require.Equal(t, "custom:/orders", spans[0].name)
	require.Equal(t, "parent-123", extracted, "the configured propagator extracts the incoming context")
}
