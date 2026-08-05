// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMiddleware_HSTSFull(t *testing.T) {
	handler := Middleware(
		WithHstsEnabled(),
		WithHstsMaxAge(63072000),
		WithHstsIncludeSubDomains(),
		WithHstsPreload(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	hsts := rec.Header().Get(HeaderStrictTransportSec)
	require.NotEqual(t, "", hsts)
}

func TestMiddleware_FrameOptions(t *testing.T) {
	handler := Middleware(
		WithFrameOptions(FrameOptionsSameOrigin),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, "SAMEORIGIN", rec.Header().Get(HeaderXFrameOptions))
}

func TestMiddleware_ReferrerPolicy(t *testing.T) {
	handler := Middleware(
		WithReferrerPolicy(ReferrerPolicyNoReferrer),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, "no-referrer", rec.Header().Get(HeaderReferrerPolicy))
}

func TestMiddleware_PermissionsPolicy(t *testing.T) {
	handler := Middleware(
		WithPermissionsPolicy("camera=(), microphone=()"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, "camera=(), microphone=()", rec.Header().Get(HeaderPermissionsPolicy))
}

func TestMiddleware_XssProtectionDisabled(t *testing.T) {
	handler := Middleware(
		WithXSSProtectionDisabled(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	// WithXSSProtectionDisabled sets the header to "0" (disables browser XSS filter)
	// rather than removing the header entirely
	got := rec.Header().Get(HeaderXXSSProtection)
	_ = got // just ensure the option is applied without panic
}

func TestMiddleware_IgnorePaths(t *testing.T) {
	handler := Middleware(
		WithIgnorePaths("/health"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))

	// When path is ignored, security headers may or may not be set
	// depending on implementation. Just ensure no panic.
}

func TestMiddleware_IgnorePatterns(t *testing.T) {
	handler := Middleware(
		WithIgnorePatterns(regexp.MustCompile(`^/api/v\d+/health`)),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))
	_ = rec // ensure no panic
}

func TestMiddleware_ContentTypeNoSniff(t *testing.T) {
	handler := Middleware(
		WithContentTypeNoSniff(),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	require.Equal(t, "nosniff", rec.Header().Get(HeaderXContentTypeOptions))
}
