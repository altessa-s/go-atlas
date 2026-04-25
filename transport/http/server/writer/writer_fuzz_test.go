// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzWriter_Read(f *testing.F) {
	f.Add(`{"key":"value"}`, "application/json")
	f.Add(`invalid`, "application/json")
	f.Add(``, "application/json")
	f.Add(`{"a":1}`, "")

	w := New()

	f.Fuzz(func(t *testing.T, body, contentType string) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		var out map[string]any
		_ = w.Read(req, &out) // Should not panic
	})
}

func FuzzWriter_Write(f *testing.F) {
	f.Add("application/json")
	f.Add("*/*")
	f.Add("")
	f.Add("text/xml")

	w := New()

	f.Fuzz(func(t *testing.T, accept string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		_ = w.Write(rec, req, "data") // Should not panic
	})
}
