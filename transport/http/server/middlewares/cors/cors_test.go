// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	m := New()
	if m.Name() != "cors" {
		t.Fatalf("Name() = %q", m.Name())
	}
}

func TestMiddleware_NoCORSRequest(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Header().Get(HeaderAccessControlAllowOrigin) != "" {
		t.Fatal("should not set CORS headers without Origin")
	}
}

func TestMiddleware_AllowAllOrigins(t *testing.T) {
	handler := Middleware(WithAllowAllOrigins())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	if rec.Header().Get(HeaderAccessControlAllowOrigin) != "*" {
		t.Fatalf("Allow-Origin = %q, want *", rec.Header().Get(HeaderAccessControlAllowOrigin))
	}
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
		if rec.Header().Get(HeaderAccessControlAllowOrigin) != "https://example.com" {
			t.Fatalf("Allow-Origin = %q", rec.Header().Get(HeaderAccessControlAllowOrigin))
		}
	})

	t.Run("not_allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://evil.com")
		handler.ServeHTTP(rec, req)
		if rec.Header().Get(HeaderAccessControlAllowOrigin) != "" {
			t.Fatal("should not set Allow-Origin for disallowed origin")
		}
	})
}

func TestMiddleware_Preflight(t *testing.T) {
	handler := Middleware(
		WithAllowAllOrigins(),
		WithAllowedMethods("GET", "POST"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for preflight")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if rec.Header().Get(HeaderAccessControlAllowMethods) == "" {
		t.Fatal("should set Allow-Methods")
	}
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

	if rec.Header().Get(HeaderAccessControlAllowCredentials) != "true" {
		t.Fatal("should set Allow-Credentials")
	}
	// With credentials, origin should be echoed, not *
	if rec.Header().Get(HeaderAccessControlAllowOrigin) != "https://example.com" {
		t.Fatalf("Allow-Origin = %q", rec.Header().Get(HeaderAccessControlAllowOrigin))
	}
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

	if rec.Header().Get(HeaderAccessControlMaxAge) != "3600" {
		t.Fatalf("Max-Age = %q", rec.Header().Get(HeaderAccessControlMaxAge))
	}
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

	if rec.Header().Get(HeaderAccessControlExposeHeaders) != "X-Custom" {
		t.Fatalf("Expose-Headers = %q", rec.Header().Get(HeaderAccessControlExposeHeaders))
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New()
	if m.Dependencies() != nil {
		t.Fatalf("Dependencies() = %v", m.Dependencies())
	}
}

func TestConstants(t *testing.T) {
	if HeaderOrigin != "Origin" {
		t.Fatal("wrong constant")
	}
	if HeaderAccessControlAllowOrigin != "Access-Control-Allow-Origin" {
		t.Fatal("wrong constant")
	}
}

func TestNew_AllowAllOriginsWithCredentials_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for AllowAllOrigins + AllowCredentials")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "insecure configuration") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	New(WithAllowAllOrigins(), WithAllowCredentials())
}

func TestNew_AllowAllOrigins_NoPanic(t *testing.T) {
	m := New(WithAllowAllOrigins())
	if m == nil {
		t.Fatal("should create middleware")
	}
}

func TestNew_AllowCredentials_NoPanic(t *testing.T) {
	m := New(WithAllowedOrigins("https://example.com"), WithAllowCredentials())
	if m == nil {
		t.Fatal("should create middleware")
	}
}
