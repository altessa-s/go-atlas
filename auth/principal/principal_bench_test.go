// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/principal"
)

func BenchmarkHasScope(b *testing.B) {
	p := principal.Principal{Scopes: []string{"a", "b", "files:read", "c", "d"}}
	b.ResetTimer()
	for b.Loop() {
		_ = p.HasScope("files:read")
	}
}
