// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package constraints

// Signed is a type constraint that permits any signed integer type, including
// named types whose underlying type is a signed integer (int, int8, int16, int32, int64).
// Use this when a generic function must support negative values or signed arithmetic.
// See also [Unsigned] and [Integer].
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned is a type constraint that permits any unsigned integer type, including
// named types whose underlying type is an unsigned integer (uint, uint8, uint16,
// uint32, uint64, uintptr). See also [Signed] and [Integer].
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// Integer is a type constraint that permits any integer type, combining both
// [Signed] and [Unsigned] constraints. This is the broadest integer constraint
// and is used extensively in generic bit manipulation and numeric functions.
type Integer interface {
	Signed | Unsigned
}

// Float is a type constraint that permits any floating-point type, including named
// types whose underlying type is float32 or float64. See also [Numbers] which
// combines this with [Integer].
type Float interface {
	~float32 | ~float64
}

// Primitive is a type constraint that permits any Go primitive type: all numeric
// types ([Float] and [Integer]), strings, and booleans, including named types with
// those underlying types. This is the broadest value-type constraint in the package.
type Primitive interface {
	Float | ~string | ~bool | Integer
}

// NumbersString is a type constraint that permits any numeric type ([Float] and
// [Integer]) as well as strings. Useful for generic functions that need to handle
// both numeric values and their string representations. Like [Primitive] but
// excludes booleans.
type NumbersString interface {
	~string | Float | Integer
}

// Numbers is a type constraint that permits any numeric type, combining [Float]
// and [Integer]. Use this for generic arithmetic, comparison, or aggregation
// functions that operate on any number. Does not include strings or booleans;
// see [Primitive] for the widest constraint.
type Numbers interface {
	Float | Integer
}
