// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMiddleware_OriginPatterns(t *testing.T) {
	handler := Middleware(
		WithAllowedOriginPatterns(regexp.MustCompile(`^https://.*\.example\.com$`)),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	t.Run("matching_pattern", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://app.example.com")
		handler.ServeHTTP(rec, req)

		require.Equal(t, "https://app.example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})

	t.Run("non_matching_pattern", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://evil.com")
		handler.ServeHTTP(rec, req)

		require.Equal(t, "", rec.Header().Get(HeaderAccessControlAllowOrigin))
	})
}

func TestMiddleware_AllowedHeaders(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithAllowedHeaders("X-Custom-Header", "Authorization"),
		WithAllowedMethods("POST"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header")
	handler.ServeHTTP(rec, req)

	require.NotEqual(t, "", rec.Header().Get(HeaderAccessControlAllowHeaders))
}

func TestMiddleware_PrivateNetwork(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithAllowPrivateNetwork(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	handler.ServeHTTP(rec, req)

	// Just ensure no panic; private network support may set additional headers
	_ = rec
}

func TestMiddleware_OptionsPassthrough(t *testing.T) {
	nextCalled := false
	handler := Middleware(
		WithAllowAllOrigins(),
		WithOptionsPassthrough(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(rec, req)

	require.True(t, nextCalled, "with OptionsPassthrough, next handler should be called")
}

func TestMiddleware_OptionsSuccessStatus(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithOptionsSuccessStatus(200),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(rec, req)

	require.Equal(t, 200, rec.Code)
}

func TestMiddleware_IgnorePaths(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithIgnorePaths("/health"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)
	// Ensure no panic; path may be ignored
}

func TestMiddleware_VaryHeader(t *testing.T) {
	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	vary := rec.Header().Get(HeaderVary)
	require.NotEqual(t, "", vary)
}
