// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
)

func TestFromContext_Empty(t *testing.T) {
	ip := clientip.FromContext(t.Context())
	require.False(t, ip.IsValid(), "expected invalid addr")
}

func TestNewContext_FromContext(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.1")
	ctx := clientip.NewContext(t.Context(), addr)
	got := clientip.FromContext(ctx)
	require.Equal(t, addr, got)
}

func TestMiddleware(t *testing.T) {
	extractor, _ := clientip.NewExtractor()
	mw := Middleware(extractor, nil)

	var gotIP netip.Addr
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = clientip.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	handler.ServeHTTP(rec, req)

	require.True(t, gotIP.IsValid(), "expected valid IP from context")
}

func TestMiddleware_InvalidRemoteAddr(t *testing.T) {
	extractor, _ := clientip.NewExtractor()
	mw := Middleware(extractor, nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "not-an-ip"
	handler.ServeHTTP(rec, req)

	require.True(t, called, "handler should still be called")
}

func TestMiddleware_RemoteAddressDiagnostics(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{name: "private by default"},
		{name: "client details", opts: []Option{WithLogClientDetails(true)}},
		{name: "raw errors", opts: []Option{WithLogRawErrors(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extractor, err := clientip.NewExtractor()
			require.NoError(t, err)
			var logs bytes.Buffer
			h := Middleware(extractor, slog.New(slog.NewJSONHandler(&logs, nil)), tc.opts...)(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					require.False(t, clientip.FromContext(r.Context()).IsValid())
					w.WriteHeader(http.StatusNoContent)
				},
			))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/private-url", nil)
			req.RemoteAddr = "private-address"
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNoContent, rec.Code)
			require.Contains(t, logs.String(), "/_unknown")
			require.NotContains(t, logs.String(), "private-url")
			if len(tc.opts) == 0 {
				require.NotContains(t, logs.String(), "private-address")
			} else {
				require.Contains(t, logs.String(), "private-address")
			}
		})
	}
}
