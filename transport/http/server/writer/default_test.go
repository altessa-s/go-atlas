// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDefault(t *testing.T) {
	d := NewDefault()
	if d == nil {
		t.Fatal("NewDefault() returned nil")
	}
}

func TestDefault_Build_Data(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, map[string]string{"key": "value"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	r, ok := resp.(*Response)
	if !ok {
		t.Fatalf("response type = %T", resp)
	}
	if r.Data == nil {
		t.Fatal("Data is nil")
	}
	if r.Error != nil {
		t.Fatal("Error should be nil for data response")
	}
}

func TestDefault_Build_Error(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, errors.New("something failed"))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	r, ok := resp.(*Response)
	if !ok {
		t.Fatalf("response type = %T", resp)
	}
	if r.Error == nil {
		t.Fatal("Error is nil")
	}
	if r.Error.Code != "" {
		t.Fatalf("Code = %q, want empty", r.Error.Code)
	}
	if r.Error.Message != "something failed" {
		t.Fatalf("Message = %q", r.Error.Message)
	}
}

type customError struct {
	code    string
	message string
	status  int
}

func (e *customError) Error() string   { return "internal: " + e.message }
func (e *customError) Code() string    { return e.code }
func (e *customError) Message() string { return e.message }
func (e *customError) HTTPStatus() int { return e.status }

func TestDefault_Build_ErrorWithInterfaces(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &customError{code: "VALIDATION", message: "invalid input", status: http.StatusBadRequest}
	resp, status := d.Build(req, err)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	r := resp.(*Response)
	if r.Error.Code != "VALIDATION" {
		t.Fatalf("Code = %q", r.Error.Code)
	}
	if r.Error.Message != "invalid input" {
		t.Fatalf("Message = %q", r.Error.Message)
	}
}

func TestDefault_Build_ErrorWithConverter(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{Code: "CUSTOM", Message: err.Error()}, http.StatusUnprocessableEntity
	}
	d := NewDefault(WithDefaultBuilderErrorConverter(converter))
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, errors.New("converted"))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", status)
	}
	r := resp.(*Response)
	if r.Error.Code != "CUSTOM" {
		t.Fatalf("Code = %q", r.Error.Code)
	}
}

func TestDefault_Build_ErrorConverterZeroStatus(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{Code: "TEST", Message: err.Error()}, 0
	}
	d := NewDefault(WithDefaultBuilderErrorConverter(converter))
	req := httptest.NewRequest("GET", "/", nil)

	_, status := d.Build(req, errors.New("test"))
	if status != http.StatusInternalServerError {
		t.Fatalf("zero status should default to 500, got %d", status)
	}
}

func TestDefault_Build_ErrorWithEmptyCode(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &emptyCodeError{}
	resp, _ := d.Build(req, err)
	r := resp.(*Response)
	if r.Error.Code != "" {
		t.Fatalf("empty Code() should fallback to empty, got %q", r.Error.Code)
	}
}

type emptyCodeError struct{}

func (e *emptyCodeError) Error() string { return "error" }
func (e *emptyCodeError) Code() string  { return "" }

func TestDefault_Build_ErrorWithEmptyMessage(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &emptyMessageError{}
	resp, _ := d.Build(req, err)
	r := resp.(*Response)
	// Should fallback to err.Error()
	if r.Error.Message != "fallback error" {
		t.Fatalf("Message = %q", r.Error.Message)
	}
}

type emptyMessageError struct{}

func (e *emptyMessageError) Error() string   { return "fallback error" }
func (e *emptyMessageError) Message() string { return "" }

func TestDefault_Build_ErrorWithZeroStatus(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &zeroStatusError{}
	_, status := d.Build(req, err)
	if status != http.StatusInternalServerError {
		t.Fatalf("zero HTTPStatus should default to 500, got %d", status)
	}
}

type zeroStatusError struct{}

func (e *zeroStatusError) Error() string   { return "error" }
func (e *zeroStatusError) HTTPStatus() int { return 0 }

