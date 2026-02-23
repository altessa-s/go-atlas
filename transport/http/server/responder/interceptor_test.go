// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorInterceptor_PassThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)
	defer ei.Flush()

	ei.WriteHeader(http.StatusOK)
	ei.Write([]byte("ok")) //nolint:errcheck
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestErrorInterceptor_InterceptsError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)

	ei.WriteHeader(http.StatusBadRequest)
	ei.Write([]byte("bad input")) //nolint:errcheck
	ei.Flush()

	if !w.called {
		t.Fatal("error writer should be called for 4xx")
	}
}

func TestErrorInterceptor_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)
	defer ei.Flush()

	ei.WriteHeader(http.StatusOK)
	ei.WriteHeader(http.StatusNotFound) // should be ignored
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestErrorInterceptor_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	ei := NewErrorInterceptor(rec, req, &mockErrorWriter{})
	defer ei.Flush()

	if ei.Unwrap() != rec {
		t.Fatal("Unwrap should return underlying writer")
	}
}

func TestErrorInterceptor_Hijack_NotSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	ei := NewErrorInterceptor(rec, req, &mockErrorWriter{})
	defer ei.Flush()

	_, _, err := ei.Hijack()
	if err == nil {
		t.Fatal("expected error for non-hijackable writer")
	}
}
