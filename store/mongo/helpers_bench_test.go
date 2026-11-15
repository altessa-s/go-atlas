// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import "testing"

func BenchmarkNextPowerOfTwo(b *testing.B) {
	i := 0
	for b.Loop() {
		nextPowerOfTwo(i % 1024)
		i++
	}
}
