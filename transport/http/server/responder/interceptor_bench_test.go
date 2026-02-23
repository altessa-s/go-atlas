// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkNewErrorInterceptor(b *testing.B) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	for b.Loop() {
		ei := NewErrorInterceptor(rec, req, w)
		ei.Flush()
	}
}

func BenchmarkErrorInterceptor_PassThrough(b *testing.B) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	for b.Loop() {
		ei := NewErrorInterceptor(rec, req, w)
		ei.WriteHeader(http.StatusOK)
		ei.Write([]byte("ok")) //nolint:errcheck
		ei.Flush()
	}
}
