// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzMiddleware(f *testing.F) {
	f.Add(int64(0), int64(10))
	f.Add(int64(100), int64(50))
	f.Add(int64(100), int64(200))

	f.Fuzz(func(t *testing.T, maxSize int64, contentLength int64) {
		if maxSize < 0 || contentLength < 0 || contentLength > 1024*1024 {
			return
		}
		mw := Middleware(maxSize)
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/fuzz", strings.NewReader(strings.Repeat("x", int(contentLength))))
		req.ContentLength = contentLength
		handler.ServeHTTP(rec, req)
	})
}
