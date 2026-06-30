// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/denylist"
)

func BenchmarkIsRevoked(b *testing.B) {
	dl := denylist.New()
	dl.Revoke("revoked")
	b.ResetTimer()
	for b.Loop() {
		_ = dl.IsRevoked("revoked")
	}
}
