// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package main

import "testing"

func BenchmarkGenerateEd25519(b *testing.B) {
	for b.Loop() {
		if _, err := generateKey(algEd25519, 0); err != nil {
			b.Fatal(err)
		}
	}
}
