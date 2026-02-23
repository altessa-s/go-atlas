// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
)

func TestNew(t *testing.T) {
	reg := prom.NewRegistry()
	m := New(WithRegisterer(reg))
	if m.Name() != "prometheus" {
		t.Fatalf("Name() = %q", m.Name())
	}
}

func TestMiddleware_Handler(t *testing.T) {
	reg := prom.NewRegistry()
	m := New(WithRegisterer(reg))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &Middleware{}
	if m.Dependencies() != nil {
		t.Fatalf("Dependencies() = %v", m.Dependencies())
	}
}

func TestRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)

	r.WriteHeader(http.StatusNotFound)
	if r.StatusCode() != http.StatusNotFound {
		t.Fatalf("StatusCode() = %d", r.StatusCode())
	}

	n, err := r.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("n = %d", n)
	}
	if r.Size() != 5 {
		t.Fatalf("Size() = %d", r.Size())
	}
}

func TestRecorder_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)
	r.WriteHeader(http.StatusOK)
	r.WriteHeader(http.StatusNotFound) // should be ignored
	if r.StatusCode() != http.StatusOK {
		t.Fatalf("StatusCode() = %d", r.StatusCode())
	}
}

func TestRecorder_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)
	if r.Unwrap() != rec {
		t.Fatal("Unwrap should return underlying writer")
	}
}

func TestConstants(t *testing.T) {
	if DefaultSubsystem != "http" {
		t.Fatalf("DefaultSubsystem = %q", DefaultSubsystem)
	}
	if DefaultMetricPrefix != "http_" {
		t.Fatalf("DefaultMetricPrefix = %q", DefaultMetricPrefix)
	}
}
