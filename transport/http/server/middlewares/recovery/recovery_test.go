// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanicRecover_NoPanic(t *testing.T) {
	mw := PanicRecover(nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestPanicRecover_Dependencies(t *testing.T) {
	m := &middleware{}
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "requestid" {
		t.Fatalf("Dependencies() = %v", deps)
	}
}
