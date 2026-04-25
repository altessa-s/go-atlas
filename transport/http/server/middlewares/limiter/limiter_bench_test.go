// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
)

func BenchmarkMiddleware_Allowed(b *testing.B) {
	lim := &mockLimiter{info: &sharedlimiter.LimitInfo{Limit: 100, Remaining: 99, Reset: 1000}}
	mw := Middleware(lim)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

type allowAllLimiter struct{}

func (a *allowAllLimiter) Limit(_ context.Context) (*sharedlimiter.LimitInfo, error) {
	return nil, nil
}

func BenchmarkMiddleware_NoInfo(b *testing.B) {
	mw := Middleware(&allowAllLimiter{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
