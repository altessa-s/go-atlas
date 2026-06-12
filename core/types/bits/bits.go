// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bits

import (
	"unsafe"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

const (
	// BitsInByte is the number of bits in a byte (8).
	// Used as the bit width for uint8 and int8 types.
	BitsInByte = 8
	// BitsInUint16 is the number of bits in a uint16 (16).
	// Used as the bit width for uint16 and int16 types.
	BitsInUint16 = 16
	// BitsInUint32 is the number of bits in a uint32 (32).
	// Used as the bit width for uint32 and int32 types.
	BitsInUint32 = 32
	// BitsInUint64 is the number of bits in a uint64 (64).
	// Used as the bit width for uint64, int64, uint, and int types.
	BitsInUint64 = 64

	// MaxBitPositionByte is the maximum zero-indexed bit position for a byte (7).
	// Bit positions for byte values must be in the range [0, MaxBitPositionByte].
	MaxBitPositionByte = BitsInByte - 1
	// MaxBitPositionUint16 is the maximum zero-indexed bit position for a uint16 (15).
	// Bit positions for uint16 values must be in the range [0, MaxBitPositionUint16].
	MaxBitPositionUint16 = BitsInUint16 - 1
	// MaxBitPositionUint32 is the maximum zero-indexed bit position for a uint32 (31).
	// Bit positions for uint32 values must be in the range [0, MaxBitPositionUint32].
	MaxBitPositionUint32 = BitsInUint32 - 1
	// MaxBitPositionUint64 is the maximum zero-indexed bit position for a uint64 (63).
	// Bit positions for uint64 values must be in the range [0, MaxBitPositionUint64].
	MaxBitPositionUint64 = BitsInUint64 - 1
)

// getBitWidth returns the bit width for the given integer type.
func getBitWidth[T constraints.Integer]() uint64 {
	var zero T
	switch any(zero).(type) {
	case uint8, int8:
		return BitsInByte
	case uint16, int16:
		return BitsInUint16
	case uint32, int32:
		return BitsInUint32
	case uint64, int64, uint, int:
		return BitsInUint64
	case uintptr:
		return uint64(unsafe.Sizeof(uintptr(0)) * 8) // #nosec G103 -- compile-time size query only, no pointer arithmetic
	default:
		return BitsInUint64
	}
}

// isValidBitPosition checks if a bit position is valid for type T.
func isValidBitPosition[T constraints.Integer](bitPosition uint64) bool {
	return bitPosition < getBitWidth[T]()
}

// isValidBitRange checks if a bit range [start, start+length) is valid for type T.
func isValidBitRange[T constraints.Integer](start, length uint64) bool {
	maxBits := getBitWidth[T]()
	return start < maxBits && start+length <= maxBits
}

// IsBitSet reports whether the bit at the specified position is set in the given value.
// The bitPosition is zero-indexed, where 0 is the least significant bit.
// Returns false if bitPosition exceeds the bit width of the type T.
// The type parameter T must satisfy [constraints.Integer].
//
// Example:
//
//	IsBitSet(uint8(5), 0) // true (bit 0 is set in 00000101)
//	IsBitSet(uint8(5), 1) // false (bit 1 is not set)
func IsBitSet[T constraints.Integer](value T, bitPosition uint64) bool {
	if !isValidBitPosition[T](bitPosition) {
		return false
	}
	return (value & (1 << bitPosition)) != 0
}

// SetBit returns value with the bit at the specified position set to 1.
// The bitPosition is zero-indexed, where 0 is the least significant bit.
// Returns the original value unchanged if bitPosition exceeds the bit width of type T.
// See also [ClearBit] and [ToggleBit] for related single-bit operations.
//
// Example:
//
//	SetBit(uint8(4), 0) // 5 (00000100 -> 00000101)
func SetBit[T constraints.Integer](value T, bitPosition uint64) T {
	if !isValidBitPosition[T](bitPosition) {
		return value
	}
	return value | (1 << bitPosition)
}

// ClearBit returns value with the bit at the specified position set to 0.
// The bitPosition is zero-indexed, where 0 is the least significant bit.
// Returns the original value unchanged if bitPosition exceeds the bit width of type T.
// See also [SetBit] and [ToggleBit] for related single-bit operations.
//
// Example:
//
//	ClearBit(uint8(5), 0) // 4 (00000101 -> 00000100)
func ClearBit[T constraints.Integer](value T, bitPosition uint64) T {
	if !isValidBitPosition[T](bitPosition) {
		return value
	}
	return value &^ (1 << bitPosition)
}

// ToggleBit returns value with the bit at the specified position flipped (0 becomes 1, 1 becomes 0).
// The bitPosition is zero-indexed, where 0 is the least significant bit.
// Returns the original value unchanged if bitPosition exceeds the bit width of type T.
// See also [SetBit] and [ClearBit] for related single-bit operations.
//
// Example:
//
//	ToggleBit(uint8(5), 0) // 4 (00000101 -> 00000100)
//	ToggleBit(uint8(5), 1) // 7 (00000101 -> 00000111)
func ToggleBit[T constraints.Integer](value T, bitPosition uint64) T {
	if !isValidBitPosition[T](bitPosition) {
		return value
	}
	return value ^ (1 << bitPosition)
}

// GetBits extracts a contiguous range of bits from value, returning them right-aligned.
// The start parameter specifies the starting bit position (zero-indexed, LSB = 0).
// The length parameter specifies how many bits to extract from the range [start, start+length).
// Returns zero if length is 0, start is out of range, or start+length exceeds the bit width of type T.
// See also [SetBits], [ClearBits], and [ToggleBits] for range-based modifications.
//
// Example:
//
//	GetBits(uint8(0b11010110), 2, 3) // 5 (extracts bits 2-4: 101)
func GetBits[T constraints.Integer](value T, start, length uint64) T {
	if length == 0 || !isValidBitRange[T](start, length) {
		return 0
	}

	mask := T((1 << length) - 1)
	return (value >> start) & mask
}

// CountSetBits returns the number of bits set to 1 (population count / Hamming weight)
// in the given value. Uses Brian Kernighan's algorithm for efficient counting.
// See also [Parity] which reports whether the count is odd or even.
//
// Example:
//
//	CountSetBits(uint8(5)) // 2 (00000101 has two 1-bits)
func CountSetBits[T constraints.Integer](value T) uint64 {
	if value == 0 {
		return 0
	}

	count := uint64(0)
	for value != 0 {
		count++
		value &= value - 1 // Clear the lowest set bit
	}
	return count
}

// CountTrailingZeros returns the number of consecutive zero bits starting from the
// least significant bit. Returns the full bit width of type T when value is zero.
// See also [CountLeadingZeros] and [FindFirstSet].
//
// Example:
//
//	CountTrailingZeros(uint8(8)) // 3 (00001000 has 3 trailing zeros)
func CountTrailingZeros[T constraints.Integer](value T) uint64 {
	if value == 0 {
		return getBitWidth[T]()
	}

	count := uint64(0)
	for (value & 1) == 0 {
		count++
		value >>= 1
	}
	return count
}

// CountLeadingZeros returns the number of consecutive zero bits starting from the
// most significant bit. Returns the full bit width of type T when value is zero.
// See also [CountTrailingZeros] and [FindLastSet].
//
// Example:
//
//	CountLeadingZeros(uint8(1)) // 7 (00000001 has 7 leading zeros)
func CountLeadingZeros[T constraints.Integer](value T) uint64 {
	if value == 0 {
		return getBitWidth[T]()
	}

	maxBits := getBitWidth[T]()
	count := uint64(0)
	for i := maxBits - 1; i < maxBits; i-- {
		if (value & (1 << i)) != 0 {
			break
		}
		count++
	}
	return count
}

// FindFirstSet returns the 1-based position of the least significant set bit.
// Returns 0 if no bits are set (value is zero). The result equals
// [CountTrailingZeros](value) + 1 for nonzero values.
// See also [FindLastSet] and [FindNextSet].
//
// Example:
//
//	FindFirstSet(uint8(12)) // 3 (00001100: first set bit at position 2, returns 3)
func FindFirstSet[T constraints.Integer](value T) uint64 {
	if value == 0 {
		return 0
	}
	return CountTrailingZeros(value) + 1
}

// FindLastSet returns the 1-based position of the most significant set bit.
// Returns 0 if no bits are set (value is zero). The result equals
// bitWidth - [CountLeadingZeros](value) for nonzero values.
// See also [FindFirstSet] and [FindNextSet].
//
// Example:
//
//	FindLastSet(uint8(12)) // 4 (00001100: last set bit at position 3, returns 4)
func FindLastSet[T constraints.Integer](value T) uint64 {
	if value == 0 {
		return 0
	}
	maxBits := getBitWidth[T]()
	return maxBits - CountLeadingZeros(value)
}

// FindNextSet returns the zero-indexed position of the next set bit strictly after
// the given position. Returns 0 if no set bit is found after the specified position
// or if after is at or beyond the last valid bit position.
// The after parameter is zero-indexed. Useful for iterating over set bits.
// See also [FindFirstSet] and [FindLastSet].
//
// Example:
//
//	FindNextSet(uint8(0b00010100), 0) // 2 (next set bit after position 0)
func FindNextSet[T constraints.Integer](value T, after uint64) uint64 {
	maxBits := getBitWidth[T]()
	if after >= maxBits-1 {
		return 0
	}

	for pos := after + 1; pos < maxBits; pos++ {
		if (value & (1 << pos)) != 0 {
			return pos
		}
	}
	return 0
}

// RotateLeft performs a circular left rotation of value by n bit positions.
// Bits that overflow the most significant end wrap around to the least significant end.
// The rotation amount is taken modulo the bit width of type T, so values of n
// greater than the bit width are handled correctly. See also [RotateRight].
//
// Example:
//
//	RotateLeft(uint8(0b10000001), 1) // 0b00000011 (3)
func RotateLeft[T constraints.Integer](value T, n uint64) T {
	maxBits := getBitWidth[T]()
	n %= maxBits // Handle n > maxBits
	if n == 0 {
		return value
	}

	// Rotate in the unsigned domain: for signed T the >> operator is
	// arithmetic and would smear the sign bit into the wrapped-around
	// positions. widthMask confines the sign-extended uint64(value) to
	// the type's bit width (and is all-ones when maxBits == 64, since a
	// 64-bit shift of a uint64 yields 0).
	widthMask := uint64(1)<<maxBits - 1
	uv := uint64(value) & widthMask
	return T(((uv << n) | (uv >> (maxBits - n))) & widthMask)
}

// RotateRight performs a circular right rotation of value by n bit positions.
// Bits that overflow the least significant end wrap around to the most significant end.
// The rotation amount is taken modulo the bit width of type T, so values of n
// greater than the bit width are handled correctly. See also [RotateLeft].
//
// Example:
//
//	RotateRight(uint8(0b10000001), 1) // 0b11000000 (192)
func RotateRight[T constraints.Integer](value T, n uint64) T {
	maxBits := getBitWidth[T]()
	n %= maxBits // Handle n > maxBits
	if n == 0 {
		return value
	}

	// See RotateLeft for why the rotation runs in the unsigned domain.
	widthMask := uint64(1)<<maxBits - 1
	uv := uint64(value) & widthMask
	return T(((uv >> n) | (uv << (maxBits - n))) & widthMask)
}

// ReverseBits returns value with all bits mirrored: the least significant bit
// becomes the most significant bit and vice versa, across the full bit width
// of type T. Internally uses [IsBitSet] and [SetBit] for each position.
//
// Example:
//
//	ReverseBits(uint8(0b10110000)) // 0b00001101 (13)
func ReverseBits[T constraints.Integer](value T) T {
	maxBits := getBitWidth[T]()
	result := T(0)

	for i := uint64(0); i < maxBits; i++ {
		if IsBitSet(value, i) {
			result = SetBit(result, maxBits-1-i)
		}
	}
	return result
}

// SetBits sets a contiguous range of bits [start, start+length) to 1 in value.
// Returns the original value unchanged if length is 0 or the range exceeds the
// bit width of type T. See also [ClearBits], [ToggleBits], and [GetBits].
//
// Example:
//
//	SetBits(uint8(0), 2, 3) // 0b00011100 (28)
func SetBits[T constraints.Integer](value T, start, length uint64) T {
	if length == 0 || !isValidBitRange[T](start, length) {
		return value
	}

	mask := T((1 << length) - 1)
	return value | (mask << start)
}

// ClearBits sets a contiguous range of bits [start, start+length) to 0 in value.
// Returns the original value unchanged if length is 0 or the range exceeds the
// bit width of type T. See also [SetBits], [ToggleBits], and [GetBits].
//
// Example:
//
//	ClearBits(uint8(0xFF), 2, 3) // 0b11100011 (227)
func ClearBits[T constraints.Integer](value T, start, length uint64) T {
	if length == 0 || !isValidBitRange[T](start, length) {
		return value
	}

	mask := T((1 << length) - 1)
	return value &^ (mask << start)
}

// ToggleBits flips a contiguous range of bits [start, start+length) in value.
// Returns the original value unchanged if length is 0 or the range exceeds the
// bit width of type T. See also [SetBits], [ClearBits], and [GetBits].
//
// Example:
//
//	ToggleBits(uint8(0b00001111), 2, 4) // 0b00110011 (51)
func ToggleBits[T constraints.Integer](value T, start, length uint64) T {
	if length == 0 || !isValidBitRange[T](start, length) {
		return value
	}

	mask := T((1 << length) - 1)
	return value ^ (mask << start)
}

// TestBits reports whether every bit in the range [start, start+length) is set to 1.
// Returns true for length == 0 (vacuous truth). Returns false if any bit in the
// range is not set or if the range exceeds the bit width of type T.
// See also [GetBits] to extract the actual bit values.
//
// Example:
//
//	TestBits(uint8(0b00011100), 2, 3) // true (bits 2-4 are all set)
func TestBits[T constraints.Integer](value T, start, length uint64) bool {
	if length == 0 {
		return true
	}
	if !isValidBitRange[T](start, length) {
		return false
	}

	mask := T((1 << length) - 1)
	expected := mask << start
	return (value & expected) == expected
}

// CreateMask creates a bitmask with length consecutive bits set to 1, starting
// from the least significant bit. Returns 0 if length is 0 or exceeds the bit
// width of type T. When length equals the full bit width, all bits are set.
// Use with [ApplyMask] to isolate specific bit ranges.
//
// Example:
//
//	CreateMask[uint8](4) // 0b00001111 (15)
func CreateMask[T constraints.Integer](length uint64) T {
	if length == 0 {
		return 0
	}

	maxBits := getBitWidth[T]()
	if length > maxBits {
		return 0
	}

	if length == maxBits {
		return ^T(0) // All bits set
	}

	return T((1 << length) - 1)
}

// ApplyMask returns the bitwise AND of value and mask, keeping only the bits
// that are set in both. Combine with [CreateMask] to isolate specific bit ranges.
//
// Example:
//
//	ApplyMask(uint8(0b11110000), uint8(0b00111100)) // 0b00110000 (48)
func ApplyMask[T constraints.Integer](value, mask T) T {
	return value & mask
}

// IsPowerOfTwo reports whether value is an exact power of two (1, 2, 4, 8, 16, ...).
// Returns false for zero and for negative values of signed types.
// Uses the efficient bit trick: a power of two has exactly one bit set.
//
// Example:
//
//	IsPowerOfTwo(8)  // true
//	IsPowerOfTwo(10) // false
func IsPowerOfTwo[T constraints.Integer](value T) bool {
	return value > 0 && (value&(value-1)) == 0
}

// Parity returns the parity bit for the given value: 1 if the number of set
// bits (as counted by [CountSetBits]) is odd, 0 if even. Useful for error
// detection in communication protocols.
//
// Example:
//
//	Parity(uint8(5)) // 0 (two bits set, even parity)
//	Parity(uint8(7)) // 1 (three bits set, odd parity)
func Parity[T constraints.Integer](value T) uint8 {
	count := CountSetBits(value)
	return uint8(count & 1) // #nosec G115 -- result of &1 is always 0 or 1
}

// SwapBits exchanges the bits at two specified zero-indexed positions in value.
// Returns the original value unchanged if either position exceeds the bit width
// of type T, or if pos1 equals pos2. When both bits have the same value (both 0
// or both 1), the result is identical to the input.
//
// Example:
//
//	SwapBits(uint8(0b00000101), 0, 2) // 0b00000101 (unchanged, both bits are 1)
//	SwapBits(uint8(0b00000001), 0, 2) // 0b00000100 (4)
func SwapBits[T constraints.Integer](value T, pos1, pos2 uint64) T {
	if !isValidBitPosition[T](pos1) || !isValidBitPosition[T](pos2) || pos1 == pos2 {
		return value
	}

	bit1 := (value & (1 << pos1)) != 0
	bit2 := (value & (1 << pos2)) != 0

	// Clear both bits first
	value &^= (1 << pos1) | (1 << pos2)

	// Set bits in swapped positions
	if bit1 {
		value |= 1 << pos2
	}
	if bit2 {
		value |= 1 << pos1
	}

	return value
}

// SignExtend interprets the low fromBits bits of value as a signed two's-complement
// integer and extends the sign bit to fill the full width of type T.
// If fromBits is 0 or exceeds the bit width of T, the original value is returned.
// For positive values (sign bit clear), high bits above fromBits are cleared.
// Uses [IsBitSet] and [CreateMask] internally.
//
// Example:
//
//	SignExtend(int8(0b00001111), 4) // -1 (extends 4-bit signed value to 8 bits)
func SignExtend[T constraints.Integer](value T, fromBits uint64) T {
	maxBits := getBitWidth[T]()
	if fromBits == 0 || fromBits > maxBits {
		return value
	}

	// Check if the sign bit is set
	signBit := fromBits - 1
	if !IsBitSet(value, signBit) {
		// Positive number, clear any bits above fromBits
		mask := CreateMask[T](fromBits)
		return value & mask
	}

	// Negative number, set all bits above fromBits
	mask := CreateMask[T](fromBits)
	return value | ^mask
}
