// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkFunc(b *testing.B) {
	for b.Loop() {
		Func("bench", func(next http.Handler) http.Handler { return next })
	}
}

func BenchmarkChain_Then(b *testing.B) {
	c := NewChain(Noop("a"), Noop("b"), Noop("c"))
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	for b.Loop() {
		c.Then(final)
	}
}

func BenchmarkResponseWriter(b *testing.B) {
	rec := httptest.NewRecorder()
	for b.Loop() {
		rw := NewResponseWriter(rec, false)
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("ok")) //nolint:errcheck
		rw.Release()
	}
}
