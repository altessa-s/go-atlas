// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback

import "testing"

func BenchmarkBehavior_IsValid(b *testing.B) {
	bh := Allow
	for b.Loop() {
		bh.IsValid()
	}
}

func BenchmarkBehavior_ShouldAllow(b *testing.B) {
	bh := Allow
	for b.Loop() {
		bh.ShouldAllow()
	}
}

func BenchmarkBehavior_String(b *testing.B) {
	bh := Allow
	var s string
	for b.Loop() {
		s = bh.String()
	}
	_ = s
}
