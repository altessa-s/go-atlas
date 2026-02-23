// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware_AllowsUnderLimit(t *testing.T) {
	mw := Middleware(1024)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", strings.NewReader("small body"))
	req.ContentLength = 10
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMiddleware_RejectsOverLimit(t *testing.T) {
	mw := Middleware(10)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", strings.NewReader(strings.Repeat("x", 100)))
	req.ContentLength = 100
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler should not be called")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code = %d, want 413", rec.Code)
	}
}

func TestMiddleware_ZeroLimit(t *testing.T) {
	mw := Middleware(0)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", strings.NewReader("body"))
	req.ContentLength = 4
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called with zero limit")
	}
}

func TestErrBodyTooLarge(t *testing.T) {
	if ErrBodyTooLarge == nil {
		t.Fatal("ErrBodyTooLarge should not be nil")
	}
	if ErrBodyTooLarge.Error() != "request body too large" {
		t.Fatalf("error = %q", ErrBodyTooLarge.Error())
	}
}
