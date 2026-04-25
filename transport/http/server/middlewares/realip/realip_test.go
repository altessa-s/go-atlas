// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

func TestFromContext_Empty(t *testing.T) {
	ip := FromContext(t.Context())
	require.False(t, ip.IsValid(), "expected invalid addr")
}

func TestNewContext_FromContext(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.1")
	ctx := NewContext(t.Context(), addr)
	got := FromContext(ctx)
	require.Equal(t, addr, got)
}

func TestMiddleware(t *testing.T) {
	extractor, _ := clientip.NewExtractor()
	mw := Middleware(extractor, nil)

	var gotIP netip.Addr
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = FromContext(r.Context())
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
