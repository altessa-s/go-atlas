// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
)

// withBodyLimitConfig returns a *httpconfig.Config carrying only the
// bodylimit middleware block. Keeps the test surface narrow so a
// regression in another middleware can't masquerade as a bodylimit bug.
func withBodyLimitConfig(t testing.TB, requireContentLength bool) *httpconfig.Config {
	t.Helper()
	return &httpconfig.Config{
		Middlewares: &httpconfig.Middlewares{
			BodyLimit: &httpconfig.BodyLimitMiddleware{
				BaseMiddleware: httpconfig.BaseMiddleware{
					EnableMixin: middlewareconfig.EnableMixin{Enabled: true},
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
func chunkedPostThroughBodyLimit(t testing.TB, b *ServerBuilder) (*httptest.ResponseRecorder, bool) {
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

// TestBodyLimitWriter_AlignsReadCap pins the writer cap the factory derives
// from the bodylimit block: Writer.Read must accept what the operator allowed
// (above writer.DefaultMaxBodySize) and reject what they did not, and a
// disabled or zero-size block leaves the server's default writer in place.
func TestBodyLimitWriter_AlignsReadCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		enabled    bool
		maxSize    int64
		bodySize   int
		wantWriter bool
		wantErr    bool
	}{
		{name: "above writer default", enabled: true, maxSize: 2 * writer.DefaultMaxBodySize,
			bodySize: int(writer.DefaultMaxBodySize) + 1, wantWriter: true},
		{name: "below writer default", enabled: true, maxSize: 1024, bodySize: 2048, wantWriter: true, wantErr: true},
		{name: "disabled", maxSize: 1024},
		{name: "zero size", enabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := withBodyLimitConfig(t, false)
			cfg.Middlewares.BodyLimit.Enabled = tc.enabled
			cfg.Middlewares.BodyLimit.MaxSize = tc.maxSize

			w := New(cfg).WithBodyLimitMiddleware().bodyLimitWriter()
			if !tc.wantWriter {
				require.Nil(t, w)
				return
			}
			require.NotNil(t, w)

			body := `{"a":"` + strings.Repeat("x", tc.bodySize-len(`{"a":""}`)) + `"}`
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			var out map[string]string
			err := w.(*writer.Writer).Read(req, &out)
			if tc.wantErr {
				require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBuild_WriterBodyLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		max        int64
		middleware bool
		install    bool
		disable    bool
		want       int
	}{
		{name: "default", want: http.StatusRequestEntityTooLarge},
		{name: "explicit unlimited", max: -1, want: http.StatusOK},
		{name: "explicit larger", max: 8 << 20, want: http.StatusOK},
		{name: "installed middleware", middleware: true, install: true, want: http.StatusOK},
		{name: "uninstalled middleware", middleware: true, want: http.StatusRequestEntityTooLarge},
		{name: "disabled middleware", middleware: true, install: true, disable: true, want: http.StatusRequestEntityTooLarge},
		{name: "explicit smaller wins", max: 1024, middleware: true, install: true, want: http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.Default()
			cfg.MaxBodySize = tc.max
			if tc.middleware {
				cfg.Middlewares = withBodyLimitConfig(t, false).Middlewares
				cfg.Middlewares.BodyLimit.MaxSize = 8 << 20
			}
			router := gorilla.New()
			b := New(&cfg).WithRouter(router)
			if tc.install {
				b.WithBodyLimitMiddleware()
			}
			if tc.disable {
				b.WithoutBodyLimitMiddleware()
			}
			srv, err := b.Build()
			require.NoError(t, err)
			srv.Handle("/body", func(rw writer.ReadWriter) {
				var out map[string]string
				if err := rw.Read(&out); err != nil {
					require.NoError(t, rw.WriteError(err))
					return
				}
				rw.ResponseWriter().WriteHeader(http.StatusOK)
			}).Methods(http.MethodPost)
			body := `{"a":"` + strings.Repeat("x", int(writer.DefaultMaxBodySize)) + `"}`
			req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(body))
			req.ContentLength = -1
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.want, rec.Code)
		})
	}
}
