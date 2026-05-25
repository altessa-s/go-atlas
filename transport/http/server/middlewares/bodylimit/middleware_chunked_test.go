// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMiddleware_ChunkedBypassesPreCheckByDefault documents the
// pre-fix legacy behavior that the audit flagged: a chunked POST
// (ContentLength == -1) slips past the size pre-check. The size cap
// is still enforced lazily by http.MaxBytesReader once the handler
// reads the body — but a handler that never reads it lets the
// chunked-drip attacker hold the connection open until ReadTimeout.
//
// We keep the default permissive for backward compatibility; the
// opt-in WithRequireContentLength test below shows how operators close
// the door.
func TestMiddleware_ChunkedBypassesPreCheckByDefault(t *testing.T) {
	called := false
	handler := Middleware(10)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	req.ContentLength = -1 // simulates Transfer-Encoding: chunked
	handler.ServeHTTP(rec, req)

	require.True(t, called,
		"default behavior: chunked POST reaches the handler — size cap is lazy via MaxBytesReader")
}

// TestMiddleware_WithRequireContentLength_RejectsChunkedPOST is the
// regression guard for the strictness option. With the option enabled,
// a chunked POST is rejected up front with 411 Length Required.
func TestMiddleware_WithRequireContentLength_RejectsChunkedPOST(t *testing.T) {
	called := false
	handler := Middleware(10, WithRequireContentLength())(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	req.ContentLength = -1
	handler.ServeHTTP(rec, req)

	require.False(t, called,
		"WithRequireContentLength must reject the request BEFORE invoking the handler — eliminates the chunked-drip attack window")
	require.Equal(t, http.StatusLengthRequired, rec.Code,
		"chunked body-bearing request without Content-Length must produce 411 Length Required")
}

// TestMiddleware_WithRequireContentLength_AllowsGETWithoutBody pins
// that the strict mode does NOT punish methods that legitimately have
// no body (GET / HEAD).
func TestMiddleware_WithRequireContentLength_AllowsGETWithoutBody(t *testing.T) {
	called := false
	handler := Middleware(10, WithRequireContentLength())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.ContentLength = -1
	handler.ServeHTTP(rec, req)

	require.True(t, called, "GET requests must NOT be rejected even when chunked")
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestMiddleware_WithRequireContentLength_AllowsPOSTWithDeclaredLength
// pins the happy path for the strict mode: a POST with a declared,
// in-bound Content-Length passes through.
func TestMiddleware_WithRequireContentLength_AllowsPOSTWithDeclaredLength(t *testing.T) {
	called := false
	handler := Middleware(1024, WithRequireContentLength())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hi"))
	req.ContentLength = 2
	handler.ServeHTTP(rec, req)

	require.True(t, called)
	require.Equal(t, http.StatusOK, rec.Code)
}
