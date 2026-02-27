// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"

	audithttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
)

func FuzzMiddleware(f *testing.F) {
	f.Add("GET", "/api/v1/items", 200)
	f.Add("POST", "/", 201)
	f.Add("DELETE", "/api/resource/123", 500)

	f.Fuzz(func(t *testing.T, method, path string, statusCode int) {
		if statusCode < 100 || statusCode > 999 {
			return
		}
		// path must be a valid URL path for httptest.NewRequest
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}

		store := memory.New()
		a, err := audit.New(store, audit.WithFlushInterval(50*time.Millisecond), audit.WithWorkers(1))
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Start(); err != nil {
			t.Fatal(err)
		}

		handler := audithttp.Middleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(statusCode)
		}))

		req, reqErr := http.NewRequest(method, "http://localhost"+path, nil)
		if reqErr != nil {
			return
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		_ = a.Shutdown(t.Context())
	})
}
