// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bits

import "testing"

func BenchmarkCountSetBits(b *testing.B) {
	for b.Loop() {
		CountSetBits(uint64(0xDEADBEEFCAFEBABE))
	}
}

func BenchmarkRotateLeft(b *testing.B) {
	for b.Loop() {
		RotateLeft(uint64(0xDEADBEEFCAFEBABE), 17)
	}
}

func BenchmarkReverseBits(b *testing.B) {
	for b.Loop() {
		ReverseBits(uint64(0xDEADBEEFCAFEBABE))
	}
}

func BenchmarkIsPowerOfTwo(b *testing.B) {
	for b.Loop() {
		IsPowerOfTwo(uint64(1 << 42))
	}
}
