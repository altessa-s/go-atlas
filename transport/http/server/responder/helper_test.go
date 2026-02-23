// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError_NoWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	err := WriteError(rec, req, errors.New("fail"), http.StatusBadRequest)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestWriteError_WithWriter(t *testing.T) {
	w := &mockErrorWriter{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(NewContext(req.Context(), w))

	err := WriteError(rec, req, errors.New("fail"), http.StatusBadRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !w.called {
		t.Fatal("writer should be called")
	}
}
