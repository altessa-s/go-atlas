// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bits

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsBitSet(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		pos  uint64
		want bool
	}{
		{"bit0 set", 0b00000101, 0, true},
		{"bit1 not set", 0b00000101, 1, false},
		{"bit2 set", 0b00000101, 2, true},
		{"out of range", 0b00000101, 8, false},
		{"zero value", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsBitSet(tt.val, tt.pos))
		})
	}
}

func TestSetBit(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		pos  uint64
		want uint8
	}{
		{"set bit0", 0b00000100, 0, 0b00000101},
		{"already set", 0b00000101, 0, 0b00000101},
		{"out of range", 0b00000101, 8, 0b00000101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SetBit(tt.val, tt.pos))
		})
	}
}

func TestClearBit(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		pos  uint64
		want uint8
	}{
		{"clear bit0", 0b00000101, 0, 0b00000100},
		{"already clear", 0b00000100, 0, 0b00000100},
		{"out of range", 0b00000101, 8, 0b00000101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClearBit(tt.val, tt.pos))
		})
	}
}

func TestToggleBit(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		pos  uint64
		want uint8
	}{
		{"toggle set bit", 0b00000101, 0, 0b00000100},
		{"toggle clear bit", 0b00000101, 1, 0b00000111},
		{"out of range", 0b00000101, 8, 0b00000101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ToggleBit(tt.val, tt.pos))
		})
	}
}

func TestGetBits(t *testing.T) {
	tests := []struct {
		name          string
		val           uint8
		start, length uint64
		want          uint8
	}{
		{"extract 3 bits from pos 2", 0b11010110, 2, 3, 0b101},
		{"zero length", 0xFF, 0, 0, 0},
		{"out of range", 0xFF, 6, 4, 0},
		{"full byte", 0b10101010, 0, 8, 0b10101010},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, GetBits(tt.val, tt.start, tt.length))
		})
	}
}

func TestCountSetBits(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint64
	}{
		{"zero", 0, 0},
		{"one bit", 1, 1},
		{"two bits", 5, 2},
		{"all bits", 0xFF, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CountSetBits(tt.val))
		})
	}
}

func TestCountTrailingZeros(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint64
	}{
		{"zero", 0, 8},
		{"bit0 set", 1, 0},
		{"bit3 set", 8, 3},
		{"0b00010100", 0b00010100, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CountTrailingZeros(tt.val))
		})
	}
}

func TestCountLeadingZeros(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint64
	}{
		{"zero", 0, 8},
		{"one", 1, 7},
		{"128", 128, 0},
		{"64", 64, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CountLeadingZeros(tt.val))
		})
	}
}

func TestFindFirstSet(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint64
	}{
		{"zero", 0, 0},
		{"12", 12, 3},
		{"1", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, FindFirstSet(tt.val))
		})
	}
}

func TestFindLastSet(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint64
	}{
		{"zero", 0, 0},
		{"12", 12, 4},
		{"1", 1, 1},
		{"128", 128, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, FindLastSet(tt.val))
		})
	}
}

func TestFindNextSet(t *testing.T) {
	tests := []struct {
		name  string
		val   uint8
		after uint64
		want  uint64
	}{
		{"next after 0", 0b00010100, 0, 2},
		{"next after 2", 0b00010100, 2, 4},
		{"none after 4", 0b00010100, 4, 0},
		{"after max", 0b00010100, 7, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, FindNextSet(tt.val, tt.after))
		})
	}
}

func TestRotateLeft(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		n    uint64
		want uint8
	}{
		{"rotate 1", 0b10000001, 1, 0b00000011},
		{"rotate 0", 0b10000001, 0, 0b10000001},
		{"rotate 8 (full)", 0b10000001, 8, 0b10000001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RotateLeft(tt.val, tt.n))
		})
	}
}

func TestRotateRight(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		n    uint64
		want uint8
	}{
		{"rotate 1", 0b10000001, 1, 0b11000000},
		{"rotate 0", 0b10000001, 0, 0b10000001},
		{"rotate 8 (full)", 0b10000001, 8, 0b10000001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RotateRight(tt.val, tt.n))
		})
	}
}

func TestReverseBits(t *testing.T) {
	tests := []struct {
		name string
		val  uint8
		want uint8
	}{
		{"0b10110000", 0b10110000, 0b00001101},
		{"0", 0, 0},
		{"0xFF", 0xFF, 0xFF},
		{"0b10000000", 0b10000000, 0b00000001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ReverseBits(tt.val))
		})
	}
}

func TestSetBits(t *testing.T) {
	tests := []struct {
		name          string
		val           uint8
		start, length uint64
		want          uint8
	}{
		{"set 3 bits from 2", 0, 2, 3, 0b00011100},
		{"zero length", 0, 0, 0, 0},
		{"out of range", 0, 6, 4, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SetBits(tt.val, tt.start, tt.length))
		})
	}
}

func TestClearBits(t *testing.T) {
	tests := []struct {
		name          string
		val           uint8
		start, length uint64
		want          uint8
	}{
		{"clear 3 bits from 2", 0xFF, 2, 3, 0b11100011},
		{"zero length", 0xFF, 0, 0, 0xFF},
		{"out of range", 0xFF, 6, 4, 0xFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClearBits(tt.val, tt.start, tt.length))
		})
	}
}

func TestToggleBits(t *testing.T) {
	tests := []struct {
		name          string
		val           uint8
		start, length uint64
		want          uint8
	}{
		{"toggle 4 bits from 2", 0b00001111, 2, 4, 0b00110011},
		{"zero length", 0xFF, 0, 0, 0xFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ToggleBits(tt.val, tt.start, tt.length))
		})
	}
}

func TestTestBits(t *testing.T) {
	tests := []struct {
		name          string
		val           uint8
		start, length uint64
		want          bool
	}{
		{"all set", 0b00011100, 2, 3, true},
		{"not all set", 0b00010100, 2, 3, false},
		{"zero length", 0, 0, 0, true},
		{"out of range", 0xFF, 6, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, TestBits(tt.val, tt.start, tt.length))
		})
	}
}

func TestCreateMask(t *testing.T) {
	tests := []struct {
		name   string
		length uint64
		want   uint8
	}{
		{"4 bits", 4, 0b00001111},
		{"0 bits", 0, 0},
		{"8 bits", 8, 0xFF},
		{"too large", 9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CreateMask[uint8](tt.length))
		})
	}
}

func TestApplyMask(t *testing.T) {
	require.Equal(t, uint8(0b00110000), ApplyMask(uint8(0b11110000), uint8(0b00111100)))
}

func TestIsPowerOfTwo(t *testing.T) {
	tests := []struct {
		val  uint8
		want bool
	}{
		{0, false},
		{1, true},
		{2, true},
		{3, false},
		{4, true},
		{8, true},
		{10, false},
		{128, true},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, IsPowerOfTwo(tt.val))
	}
}

func TestParity(t *testing.T) {
	tests := []struct {
		val  uint8
		want uint8
	}{
		{5, 0},  // 2 bits set
		{7, 1},  // 3 bits set
		{0, 0},  // 0 bits set
		{15, 0}, // 4 bits set
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, Parity(tt.val))
	}
}

func TestSwapBits(t *testing.T) {
	tests := []struct {
		name       string
		val        uint8
		pos1, pos2 uint64
		want       uint8
	}{
		{"swap different bits", 0b00000001, 0, 2, 0b00000100},
		{"swap same bits", 0b00000101, 0, 2, 0b00000101},
		{"same position", 0b00000101, 0, 0, 0b00000101},
		{"out of range", 0b00000101, 0, 8, 0b00000101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SwapBits(tt.val, tt.pos1, tt.pos2))
		})
	}
}

func TestSignExtend(t *testing.T) {
	tests := []struct {
		name     string
		val      int8
		fromBits uint64
		want     int8
	}{
		{"negative 4-bit", 0b00001111, 4, -1},
		{"positive 4-bit", 0b00000111, 4, 7},
		{"zero fromBits", 5, 0, 5},
		{"too large fromBits", 5, 9, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SignExtend(tt.val, tt.fromBits))
		})
	}
}
