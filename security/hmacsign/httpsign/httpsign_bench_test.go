// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpsign_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/security/hmacsign"
	"github.com/altessa-s/go-atlas/security/hmacsign/httpsign"
)

func BenchmarkMiddleware(b *testing.B) {
	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte("whsec_bench"))
	body := `{"event":"payment.succeeded","id":"evt_bench"}`
	header := hmacsign.NewSigner(hmacsign.GitHub(), []byte("whsec_bench")).Sign([]byte(body))
	handler := httpsign.Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
		req.Header.Set(v.HeaderName(), header)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}
