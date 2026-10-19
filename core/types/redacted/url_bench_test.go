// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

func BenchmarkURL_String_WithCredentials(b *testing.B) {
	u := redacted.URL("http://svc:secret@proxy.internal:3128")
	for b.Loop() {
		_ = u.String()
	}
}

func BenchmarkURL_String_Plain(b *testing.B) {
	u := redacted.URL("http://proxy.internal:3128")
	for b.Loop() {
		_ = u.String()
	}
}
