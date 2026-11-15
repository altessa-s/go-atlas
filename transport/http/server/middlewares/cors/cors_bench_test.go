// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkMiddleware_NoCORS(b *testing.B) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_WithCORS(b *testing.B) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	req.Header.Set("Origin", "https://example.com")
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_PreflightRestricted(b *testing.B) {
	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
		WithAllowedMethods("GET", "POST"),
		WithAllowedHeaders("Content-Type", "X-Request-ID"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("OPTIONS", "/bench", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type, x-request-id")

	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_Preflight(b *testing.B) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("OPTIONS", "/bench", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
