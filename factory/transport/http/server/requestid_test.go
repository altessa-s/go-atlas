// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

// White-box access is needed to exercise the configured middleware without starting a server.
func TestRequestIDMiddlewareGenerateIfMissing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		generate bool
		want     int
	}{
		{name: "enabled generates", generate: true, want: http.StatusOK},
		{name: "disabled rejects a missing id", generate: false, want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.DefaultRequestIDMiddleware()
			cfg.Enabled = true
			cfg.GenerateIfMissing = tc.generate
			b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{RequestId: &cfg}}).WithRequestIDMiddleware()
			require.Len(t, b.configMW, 1)

			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			b.configMW[0].Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, r)
			require.Equal(t, tc.want, rec.Code)
		})
	}
}

// The middleware reads and returns the request ID under the configured
// header, X-Request-ID by default.
func TestRequestIDMiddlewareHeaderName(t *testing.T) {
	t.Parallel()
	const id = "550e8400-e29b-41d4-a716-446655440000"
	for _, tc := range []struct {
		name       string
		headerName string // empty keeps the default
		header     string
	}{
		{name: "default header", header: "X-Request-ID"},
		{name: "configured header", headerName: "X-Correlation-ID", header: "X-Correlation-ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.DefaultRequestIDMiddleware()
			cfg.Enabled = true
			cfg.GenerateIfMissing = false
			if tc.headerName != "" {
				cfg.HeaderName = tc.headerName
			}
			b := New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{RequestId: &cfg}}).WithRequestIDMiddleware()
			require.Len(t, b.configMW, 1)

			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.Header.Set(tc.header, id)
			b.configMW[0].Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, r)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, id, rec.Header().Get(tc.header))
		})
	}
}
