// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New()
	require.Equal(t, "securityheaders", m.Name())
}

func TestMiddleware_DefaultHeaders(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{HeaderXContentTypeOptions, "nosniff"},
		{HeaderXFrameOptions, "DENY"},
		{HeaderReferrerPolicy, "strict-origin-when-cross-origin"},
		{HeaderXXSSProtection, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := rec.Header().Get(tt.header)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMiddleware_HSTS(t *testing.T) {
	handler := Middleware(WithHstsEnabled())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	hsts := rec.Header().Get(HeaderStrictTransportSec)
	require.NotEqual(t, "", hsts)
}

func TestMiddleware_NoHSTSByDefault(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, "", rec.Header().Get(HeaderStrictTransportSec))
}

func TestMiddleware_CSP(t *testing.T) {
	policy := "default-src 'self'"
	handler := Middleware(WithContentSecurityPolicy(policy))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, policy, rec.Header().Get(HeaderContentSecurityPolicy))
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	require.Nil(t, deps)
}
