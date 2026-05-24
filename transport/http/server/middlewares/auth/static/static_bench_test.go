// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"
)

func generateTokens(count int) map[string]any {
	tokens := make(map[string]any, count)
	for i := range count {
		tokens["token_"+strconv.Itoa(i)] = userInfo{
			ID:    "user_" + strconv.Itoa(i),
			Role:  "role_" + strconv.Itoa(i%3),
			Email: "user" + strconv.Itoa(i) + "@example.com",
		}
	}
	return tokens
}

func BenchmarkAuthFunc_Hit(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	fn := static.AuthFunc(store)
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_, _ = fn.Authenticate(ctx, "token_500")
	}
}

func BenchmarkHTTPIntegration_BearerToken(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	middleware := auth.Middleware(auth.WithAuthFunc(static.AuthFunc(store)))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer token_500")

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkHTTPIntegration_APIKey(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	extractor := auth.ExtractTokenFromHeader("X-API-Key", func(v string) (string, error) { return v, nil })

	middleware := auth.Middleware(
		auth.WithTokenExtractor(extractor),
		auth.WithAuthFunc(static.AuthFunc(store)),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("X-API-Key", "token_500")

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkHTTPIntegration_Parallel(b *testing.B) {
	store := static.NewInMemoryStore(static.WithInitialTokens(generateTokens(1000)))
	middleware := auth.Middleware(auth.WithAuthFunc(static.AuthFunc(store)))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
			req.Header.Set("Authorization", "Bearer token_"+strconv.Itoa(i%1000))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			i++
		}
	})
}
