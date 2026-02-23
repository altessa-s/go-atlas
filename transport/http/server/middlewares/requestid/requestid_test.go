// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/requestid"
)

func TestRequestId_GeneratesIfMissing(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id == "" {
			t.Fatal("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Header().Get(gen.HeaderName()) == "" {
		t.Fatal("expected request ID in response header")
	}
}

func TestRequestId_UsesExisting(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	existingID := "550e8400-e29b-41d4-a716-446655440000"

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id != existingID {
			t.Fatalf("id = %q, want %q", id, existingID)
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(gen.HeaderName(), existingID)
	handler.ServeHTTP(rec, req)
}

func TestRequestId_SkipsOptions(t *testing.T) {
	gen := requestid.NewGenerator()
	mw := RequestId(gen)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called for OPTIONS")
	}
}

func TestFromContext_Empty(t *testing.T) {
	id := FromContext(t.Context())
	if id != "" {
		t.Fatalf("expected empty, got %q", id)
	}
}

func TestNewContext(t *testing.T) {
	ctx := NewContext(t.Context(), "test-id")
	if FromContext(ctx) != "test-id" {
		t.Fatal("expected test-id")
	}
}
