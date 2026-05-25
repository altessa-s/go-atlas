// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

// withBodyLimitConfig returns a *config.Http carrying only the
// bodylimit middleware block. Keeps the test surface narrow so a
// regression in another middleware can't masquerade as a bodylimit bug.
func withBodyLimitConfig(t *testing.T, requireContentLength bool) *config.Http {
	t.Helper()
	return &config.Http{
		Middlewares: &config.MiddlewaresConfig{
			BodyLimit: &config.HttpInterBodyLimitConfig{
				BaseHttpMiddlewareConfig: config.BaseHttpMiddlewareConfig{
					EnableMixin: config.EnableMixin{Enabled: true},
				},
				MaxSize:              1024,
				RequireContentLength: requireContentLength,
			},
		},
	}
}

// chunkedPostThroughBodyLimit drives a chunked POST through the
// builder-assembled bodylimit middleware. Returns the recorder so the
// caller can assert on status and a flag for whether the inner handler
// fired (proves the rejection happens BEFORE the handler — eliminating
// the chunked-drip attack window).
func chunkedPostThroughBodyLimit(t *testing.T, b *ServerBuilder) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	b.WithBodyLimitMiddleware()
	require.Len(t, b.configMW, 1, "expected exactly one bodylimit middleware on the chain")

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := b.configMW[0].Handler(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	req.ContentLength = -1 // simulates Transfer-Encoding: chunked
	handler.ServeHTTP(rec, req)
	return rec, called
}

// TestWithBodyLimitMiddleware_RequireContentLength_True is the
// strict-path regression: when the YAML knob is true, the factory must
// thread bodylimit.WithRequireContentLength() into bodylimitmw.New so
// chunked POSTs without Content-Length are rejected up front with 411.
// A regression here (e.g. the option silently dropped) would expose
// every operator-enabled bodylimit to the chunked-drip attack again.
func TestWithBodyLimitMiddleware_RequireContentLength_True(t *testing.T) {
	t.Parallel()

	b := New(withBodyLimitConfig(t, true))
	rec, called := chunkedPostThroughBodyLimit(t, b)

	require.False(t, called,
		"strict mode: chunked POST must be rejected BEFORE the handler")
	require.Equal(t, http.StatusLengthRequired, rec.Code,
		"strict mode: chunked POST without Content-Length must produce 411 Length Required")
}

// TestWithBodyLimitMiddleware_RequireContentLength_False is the
// opt-out path: operators with legitimate streaming clients can flip
// the YAML knob to false and chunked POSTs flow through to the handler
// as before — the lazy MaxBytesReader still bounds the body once the
// handler reads it.
func TestWithBodyLimitMiddleware_RequireContentLength_False(t *testing.T) {
	t.Parallel()

	b := New(withBodyLimitConfig(t, false))
	rec, called := chunkedPostThroughBodyLimit(t, b)

	require.True(t, called,
		"opt-out mode: chunked POST must reach the handler — operator accepted the lazy MaxBytesReader bound")
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestWithBodyLimitMiddleware_DisabledSkipsAppend pins the "enabled
// gate wins over RequireContentLength" rule: if the operator left the
// whole bodylimit block disabled, the factory must NOT register a
// middleware at all — even if RequireContentLength is true. Otherwise
// flipping enabled→false would silently still reject chunked POSTs,
// surprising operators who think disabled means "no middleware".
func TestWithBodyLimitMiddleware_DisabledSkipsAppend(t *testing.T) {
	t.Parallel()

	cfg := withBodyLimitConfig(t, true)
	cfg.Middlewares.BodyLimit.Enabled = false

	b := New(cfg)
	b.WithBodyLimitMiddleware()

	require.Empty(t, b.configMW,
		"disabled bodylimit must not register any middleware, regardless of RequireContentLength")
}
