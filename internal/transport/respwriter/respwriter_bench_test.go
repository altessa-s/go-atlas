// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package respwriter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/internal/transport/respwriter"
)

func BenchmarkRecorder_Write(b *testing.B) {
	rec := httptest.NewRecorder()
	payload := []byte(`{"ok":true}`)
	for b.Loop() {
		w := respwriter.NewRecorder(rec, true, 1<<20)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		w.Release()
		rec.Body.Reset()
	}
}

func BenchmarkMarkStructured(b *testing.B) {
	w := wrapper{wrapper{&marker{ResponseWriter: httptest.NewRecorder()}}}
	for b.Loop() {
		respwriter.MarkStructured(w)
	}
}
