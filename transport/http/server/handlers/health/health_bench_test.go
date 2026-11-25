// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

func BenchmarkK8sHealtz(b *testing.B) {
	w := writer.New()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/healthz", nil)
		rw, cleanup := writer.NewReadWriter(rec, req, w)
		K8sHealtz(rw)
		cleanup()
	}
}

func BenchmarkK8sReadyz(b *testing.B) {
	w := writer.New()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/readyz", nil)
		rw, cleanup := writer.NewReadWriter(rec, req, w)
		K8sReadyz(rw)
		cleanup()
	}
}
