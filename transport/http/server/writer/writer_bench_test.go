// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}

func BenchmarkWriter_Write(b *testing.B) {
	w := New()
	data := map[string]string{"key": "value"}

	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		_ = w.Write(rec, req, data)
	}
}

func BenchmarkWriter_Read(b *testing.B) {
	w := New()
	body := `{"name":"test","value":"bench"}`

	for b.Loop() {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		var out map[string]string
		_ = w.Read(req, &out)
	}
}

func BenchmarkWriter_WriteError(b *testing.B) {
	w := New()
	testErr := &testError{code: "TEST", message: "test error", status: 400}

	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		_ = w.WriteError(rec, req, testErr, 400)
	}
}

func BenchmarkNewReadWriter(b *testing.B) {
	w := New()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		rw := NewReadWriter(rec, req, w)
		rw.Release()
	}
}

type testError struct {
	code    string
	message string
	status  int
}

func (e *testError) Error() string   { return e.message }
func (e *testError) Code() string    { return e.code }
func (e *testError) Message() string { return e.message }
func (e *testError) HTTPStatus() int { return e.status }
