// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bits

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzRotateLeftRight(f *testing.F) {
	f.Add(uint8(0b10110011), uint64(3))
	f.Add(uint8(0), uint64(0))
	f.Add(uint8(0xFF), uint64(8))

	f.Fuzz(func(t *testing.T, val uint8, n uint64) {
		n = n % 8
		got := RotateRight(RotateLeft(val, n), n)
		require.Equal(t, val, got)
	})
}

func FuzzReverseBits(f *testing.F) {
	f.Add(uint8(0b10110000))
	f.Add(uint8(0))
	f.Add(uint8(0xFF))

	f.Fuzz(func(t *testing.T, val uint8) {
		got := ReverseBits(ReverseBits(val))
		require.Equal(t, val, got)
	})
}
