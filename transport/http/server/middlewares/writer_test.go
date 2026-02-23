// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWriter_StatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.WriteHeader(http.StatusNotFound)
	if rw.StatusCode() != http.StatusNotFound {
		t.Fatalf("StatusCode() = %d", rw.StatusCode())
	}
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	if rw.StatusCode() != http.StatusOK {
		t.Fatalf("StatusCode() = %d", rw.StatusCode())
	}
}

func TestResponseWriter_CaptureBody(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, true)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	if rw.BodyString() != "hello" {
		t.Fatalf("Body() = %q", rw.BodyString())
	}
}

func TestResponseWriter_NoCaptureBody(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	if len(rw.Body()) != 0 {
		t.Fatal("should not capture body")
	}
}

func TestResponseWriter_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.WriteHeader(http.StatusOK)
	rw.WriteHeader(http.StatusNotFound)
	if rw.StatusCode() != http.StatusOK {
		t.Fatalf("second WriteHeader should be ignored, got %d", rw.StatusCode())
	}
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	if rw.Unwrap() != rec {
		t.Fatal("Unwrap should return underlying writer")
	}
}
