// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// nopHandler never writes, so the recorder can be reused across iterations.
var nopHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func BenchmarkHandler_Allowed(b *testing.B) {
	handler := Middleware(newTestRegistry())(nopHandler)
	req := requestWithIP("GET", "/allowed", netip.MustParseAddr("10.1.1.1"))
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkHandler_IgnoredPath(b *testing.B) {
	mw := New(newTestRegistry(), WithIgnorePaths("/skip"))
	handler := mw.Handler(nopHandler)
	req := requestWithIP("GET", "/skip", netip.MustParseAddr("10.1.1.1"))
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(rec, req)
	}
}
