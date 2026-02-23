// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	w := New()
	if w == nil {
		t.Fatal("New() returned nil")
	}
}

func TestWriter_Write_NilResponseWriter(t *testing.T) {
	w := New()
	req := httptest.NewRequest("GET", "/", nil)
	err := w.Write(nil, req, "data")
	if !errors.Is(err, ErrNilResponseWriter) {
		t.Fatalf("expected ErrNilResponseWriter, got %v", err)
	}
}

func TestWriter_Write_NilRequest(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	err := w.Write(rec, nil, "data")
	if !errors.Is(err, ErrNilRequest) {
		t.Fatalf("expected ErrNilRequest, got %v", err)
	}
}

func TestWriter_Write_JSON(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")

	err := w.Write(rec, req, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
}

func TestWriter_Write_DefaultCodec(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	// No Accept header - should use default codec

	err := w.Write(rec, req, "hello")
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWriter_Read_NilRequest(t *testing.T) {
	w := New()
	var out struct{}
	err := w.Read(nil, &out)
	if !errors.Is(err, ErrNilRequest) {
		t.Fatalf("expected ErrNilRequest, got %v", err)
	}
}

func TestWriter_Read_NilOutput(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
	err := w.Read(req, nil)
	if !errors.Is(err, ErrNilOutput) {
		t.Fatalf("expected ErrNilOutput, got %v", err)
	}
}

func TestWriter_Read_JSON(t *testing.T) {
	w := New()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := w.Read(req, &out)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if out["name"] != "test" {
		t.Fatalf("name = %q", out["name"])
	}
}

func TestWriter_Read_DefaultContentType(t *testing.T) {
	w := New()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	// No Content-Type header - should use default codec

	var out map[string]string
	err := w.Read(req, &out)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWriter_Read_UnsupportedContentType(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader("data"))
	req.Header.Set("Content-Type", "application/unsupported-format-xyz")

	var out map[string]string
	err := w.Read(req, &out)
	if err == nil {
		t.Fatal("expected error for unsupported content type")
	}
}

func TestWriter_WriteError(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	err := w.WriteError(rec, req, errors.New("something failed"), http.StatusBadRequest)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWriter_WriteError_DefaultStatus(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	err := w.WriteError(rec, req, errors.New("fail"))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestWriter_Read_MaxBodySize(t *testing.T) {
	w := New(WithMaxBodySize(5))
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"this is a very long body that exceeds limit"}`))
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := w.Read(req, &out)
	if err == nil {
		t.Fatal("expected body size limit error")
	}
}

func TestNewReadWriter(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	if rw == nil {
		t.Fatal("NewReadWriter returned nil")
	}
	if rw.Request() != req {
		t.Fatal("Request() mismatch")
	}
	if rw.ResponseWriter() != rec {
		t.Fatal("ResponseWriter() mismatch")
	}
	rw.Release()
}

func TestWriter_NewReadWriter(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := w.NewReadWriter(rec, req)
	if rw == nil {
		t.Fatal("returned nil")
	}
	rw.Release()
}

func TestReadWriter_Write(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	err := rw.Write(map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestReadWriter_Read(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	var out map[string]string
	err := rw.Read(&out)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestReadWriter_WriteError(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	err := rw.WriteError(errors.New("fail"), http.StatusBadRequest)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
}

func TestReadWriter_Release_ClearsFields(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	rw.Release()
	// After release, the underlying struct fields should be nil.
	// We can't check directly, but at least verify no panic.
}

func TestSentinelErrors_NotNil(t *testing.T) {
	sentinels := []error{
		ErrNoCodecAvailable,
		ErrReadBody,
		ErrUnmarshal,
		ErrNilResponseWriter,
		ErrNilRequest,
		ErrNilOutput,
		ErrBodySizeLimitExceeded,
	}
	for _, err := range sentinels {
		if err == nil {
			t.Fatal("sentinel error is nil")
		}
		if err.Error() == "" {
			t.Fatalf("empty message: %v", err)
		}
	}
}

func TestWriter_Write_WithAcceptWildcard(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")

	err := w.Write(rec, req, "data")
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestWriter_Read_EmptyBody(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(""))

	var out map[string]string
	err := w.Read(req, &out)
	// Empty body should produce an unmarshal error
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}
