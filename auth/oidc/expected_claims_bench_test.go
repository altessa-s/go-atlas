// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

func BenchmarkClaimMatches(b *testing.B) {
	b.Run("bool", func(b *testing.B) {
		for b.Loop() {
			_ = claimMatches(true, true)
		}
	})
	b.Run("array", func(b *testing.B) {
		groups := []any{"a", "b", "c", "d"}
		for b.Loop() {
			_ = claimMatches(groups, "d")
		}
	})
}
