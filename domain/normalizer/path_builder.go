// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"strconv"
	"sync"
)

const (
	// DefaultPathBuilderCapacity is the default capacity for path builder.
	DefaultPathBuilderCapacity = 64
	// MaxPathBuilderCapacity is the maximum capacity we'll keep in the pool.
	MaxPathBuilderCapacity = 512
	// VerySmallStringThreshold is the threshold for byte operation optimization.
	VerySmallStringThreshold = 20
)

// pathBuilderBytes uses byte slices for efficient string building.
// It is pooled to reduce allocations during path construction.
type pathBuilderBytes struct {
	buf []byte
}

var pathBuilderBytesPool = sync.Pool{
	New: func() any {
		return &pathBuilderBytes{
			buf: make([]byte, 0, DefaultPathBuilderCapacity),
		}
	},
}

// buildArrayPath creates an array path using byte operations.
// Uses pooled buffers for efficient string construction.
//
// Example:
//
//	buildArrayPath("parent", 5) // returns "parent[5]"
//	buildArrayPath("", 5)       // returns "[5]"
func buildArrayPath(parentPath string, index int) string {
	pb, ok := pathBuilderBytesPool.Get().(*pathBuilderBytes)
	if !ok {
		pb = &pathBuilderBytes{}
	}
	defer func() {
		pb.buf = pb.buf[:0]
		if cap(pb.buf) <= MaxPathBuilderCapacity {
			pathBuilderBytesPool.Put(pb)
		}
	}()

	// Estimate capacity needed
	const bracketsLength = 2 // for "[" and "]"
	needed := len(parentPath) + bracketsLength + lengthOfInt(index)
	if cap(pb.buf) < needed {
		pb.buf = make([]byte, 0, needed)
	}

	if parentPath != "" {
		pb.buf = append(pb.buf, parentPath...)
	}
	pb.buf = append(pb.buf, '[')
	pb.buf = strconv.AppendInt(pb.buf, int64(index), 10)
	pb.buf = append(pb.buf, ']')

	return string(pb.buf)
}

// buildFieldPath creates a field path using byte operations.
// For small concatenations, direct string concat is used for better performance.
//
// Example:
//
//	buildFieldPath("parent", "field") // returns "parent.field"
//	buildFieldPath("", "field")       // returns "field"
func buildFieldPath(parentPath string, fieldName string) string {
	if parentPath == "" {
		return fieldName
	}

	// For very small concatenations, direct string concat is faster
	if len(parentPath)+len(fieldName) < VerySmallStringThreshold {
		return parentPath + "." + fieldName
	}

	pb, ok := pathBuilderBytesPool.Get().(*pathBuilderBytes)
	if !ok {
		pb = &pathBuilderBytes{}
	}
	defer func() {
		pb.buf = pb.buf[:0]
		if cap(pb.buf) <= MaxPathBuilderCapacity {
			pathBuilderBytesPool.Put(pb)
		}
	}()

	const dotLength = 1 // for "."
	needed := len(parentPath) + dotLength + len(fieldName)
	if cap(pb.buf) < needed {
		pb.buf = make([]byte, 0, needed)
	}

	pb.buf = append(pb.buf, parentPath...)
	pb.buf = append(pb.buf, '.')
	pb.buf = append(pb.buf, fieldName...)

	return string(pb.buf)
}

// Constants for lengthOfInt function to avoid magic numbers
const (
	Ten             = 10
	Hundred         = 100
	Thousand        = 1000
	TenThousand     = 10000
	HundredThousand = 100000
	Million         = 1000000
	TenMillion      = 10000000
	HundredMillion  = 100000000
	Billion         = 1000000000

	// Digit counts for lengthOfInt return values
	OneDigit    = 1
	TwoDigits   = 2
	ThreeDigits = 3
	FourDigits  = 4
	FiveDigits  = 5
	SixDigits   = 6
	SevenDigits = 7
	EightDigits = 8
	NineDigits  = 9
	TenDigits   = 10
)

// lengthOfInt returns the number of digits in an integer.
// Handles negative numbers by taking the absolute value.
func lengthOfInt(n int) int {
	if n < 0 {
		n = -n
	}
	switch {
	case n < Ten:
		return OneDigit
	case n < Hundred:
		return TwoDigits
	case n < Thousand:
		return ThreeDigits
	case n < TenThousand:
		return FourDigits
	case n < HundredThousand:
		return FiveDigits
	case n < Million:
		return SixDigits
	case n < TenMillion:
		return SevenDigits
	case n < HundredMillion:
		return EightDigits
	case n < Billion:
		return NineDigits
	default:
		return TenDigits
	}
}
