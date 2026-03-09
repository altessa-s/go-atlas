// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestNew_NilTracer(t *testing.T) {
	m := New(nil)
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.Name() != "tracing" {
		t.Fatalf("Name() = %q", m.Name())
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New(nil)
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "realip" {
		t.Fatalf("Dependencies() = %v", deps)
	}
}

func TestTracingHandler(t *testing.T) {
	handler := TracingHandler(tracing.Noop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestWrapHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WrapHandler(tracing.Noop(), inner)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestExtractClientIP_FromRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	ip := extractClientIP(req)
	if ip != "192.168.1.1" {
		t.Fatalf("ip = %q", ip)
	}
}

func TestDefaultSpanName(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/users", nil)
	name := defaultSpanName(req)
	if name != "POST /api/users" {
		t.Fatalf("name = %q", name)
	}
}
