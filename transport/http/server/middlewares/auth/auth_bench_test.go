// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkMiddleware_ValidToken(b *testing.B) {
	m := New(WithAuthFunc(AuthenticateFunc(func(_ context.Context, token string) (any, error) {
		if token == "valid_token" {
			return testUserInfo{ID: "user1", Role: "admin"}, nil
		}
		return nil, ErrUnauthorized
	})))

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid_token")

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_InvalidToken(b *testing.B) {
	m := New(WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
		return nil, ErrUnauthorized
	})))

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_IgnoredPath(b *testing.B) {
	m := New(
		WithIgnorePaths("/health"),
		WithAuthFunc(AuthenticateFunc(func(context.Context, string) (any, error) {
			return testUserInfo{ID: "user1"}, nil
		})),
	)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkExtractBearerToken(b *testing.B) {
	extractor := ExtractBearerToken()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc123xyz")

	b.ResetTimer()
	for b.Loop() {
		_, _ = extractor.ExtractToken(req)
	}
}

func BenchmarkExtractTokenFromHeader(b *testing.B) {
	extractor := ExtractTokenFromHeader("X-API-Key", func(value string) (string, error) {
		return value, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret_key_123")

	b.ResetTimer()
	for b.Loop() {
		_, _ = extractor.ExtractToken(req)
	}
}

func BenchmarkFromContext(b *testing.B) {
	ctx := context.WithValue(context.Background(), authContextKey, testUserInfo{ID: "user1", Role: "admin"})
	b.ResetTimer()
	for b.Loop() {
		_ = FromContext(ctx)
	}
}

func BenchmarkMiddleware_Parallel(b *testing.B) {
	m := New(WithAuthFunc(AuthenticateFunc(func(_ context.Context, token string) (any, error) {
		if token == "valid_token" {
			return testUserInfo{ID: "user1", Role: "admin"}, nil
		}
		return nil, ErrUnauthorized
	})))

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		for pb.Next() {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}
	})
}
