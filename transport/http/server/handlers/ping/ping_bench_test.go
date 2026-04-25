// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ping

import (
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

func BenchmarkHandler(b *testing.B) {
	w := writer.New()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/ping", nil)
		rw := writer.NewReadWriter(rec, req, w)
		Handler(rw)
		rw.Release()
	}
}
