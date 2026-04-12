// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New()
	require.Equal(t, "cors", m.Name())
}

func TestMiddleware_NoCORSRequest(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, "", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_AllowAllOrigins(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "*", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_SpecificOrigin(t *testing.T) {
	handler := Middleware(WithAllowedOrigins("https://example.com"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rec, req)
		require.Equal(t, "https://example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})

	t.Run("not_allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		handler.ServeHTTP(rec, req)
		require.Equal(t, "", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})
}

func TestMiddleware_Preflight(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithAllowedMethods("GET", "POST"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Fail(t, "next handler should not be called for preflight")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotEqual(t, "", rec.Header().Get(HeaderAccessControlAllowMethods))
}

func TestMiddleware_Credentials(t *testing.T) {
	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
		WithAllowCredentials(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "true", rec.Header().Get(HeaderAccessControlAllowCredentials))
	// With credentials, origin should be echoed, not *
	require.Equal(t, "https://example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
}

func TestMiddleware_MaxAge(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithMaxAge(3600),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "3600", rec.Header().Get(HeaderAccessControlMaxAge))
}

func TestMiddleware_ExposedHeaders(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithExposedHeaders("X-Custom"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	require.Equal(t, "X-Custom", rec.Header().Get(HeaderAccessControlExposeHeaders))
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New()
	require.Nil(t, m.Dependencies())
}

func TestNew_AllowAllOriginsWithCredentials_Panics(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r)
		msg, ok := r.(string)
		require.True(t, ok && strings.Contains(msg, "insecure configuration"), "unexpected panic: %v", r)
	}()

	New(WithAllowAllOrigins(), WithAllowCredentials())
}

func TestNew_AllowAllOrigins_NoPanic(t *testing.T) {
	m := New(WithAllowAllOrigins())
	require.NotNil(t, m)
}

func TestNew_AllowCredentials_NoPanic(t *testing.T) {
	m := New(WithAllowedOrigins("https://example.com"), WithAllowCredentials())
	require.NotNil(t, m)
}
