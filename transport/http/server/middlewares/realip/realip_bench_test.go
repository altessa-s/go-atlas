// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

func BenchmarkMiddleware(b *testing.B) {
	extractor, _ := clientip.NewExtractor()
	mw := Middleware(extractor, nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
