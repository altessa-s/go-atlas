// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package constraints_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// The constraints package only declares generic type sets. There is no
// runtime behavior to assert, so the tests below exercise each
// constraint by instantiating a trivial generic helper with every member
// type. A successful compile proves the constraint accepts the type; the
// test function then performs a cheap runtime check so `go test` reports
// a PASS line.

func sum[T constraints.Numbers](a, b T) T { return a + b }

func identity[T constraints.Primitive](v T) T { return v }

func firstSigned[T constraints.Signed](vs ...T) T {
	var zero T
	if len(vs) == 0 {
		return zero
	}
	return vs[0]
}

func firstUnsigned[T constraints.Unsigned](vs ...T) T {
	var zero T
	if len(vs) == 0 {
		return zero
	}
	return vs[0]
}

func firstInteger[T constraints.Integer](vs ...T) T {
	var zero T
	if len(vs) == 0 {
		return zero
	}
	return vs[0]
}

func firstFloat[T constraints.Float](vs ...T) T {
	var zero T
	if len(vs) == 0 {
		return zero
	}
	return vs[0]
}

func firstNumbersString[T constraints.NumbersString](vs ...T) T {
	var zero T
	if len(vs) == 0 {
		return zero
	}
	return vs[0]
}

func TestSigned(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1, firstSigned[int](1, 2))
	require.Equal(t, int8(1), firstSigned[int8](1))
	require.Equal(t, int16(1), firstSigned[int16](1))
	require.Equal(t, int32(1), firstSigned[int32](1))
	require.Equal(t, int64(1), firstSigned[int64](1))
}

func TestUnsigned(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint(1), firstUnsigned[uint](1))
	require.Equal(t, uint8(1), firstUnsigned[uint8](1))
	require.Equal(t, uint16(1), firstUnsigned[uint16](1))
	require.Equal(t, uint32(1), firstUnsigned[uint32](1))
	require.Equal(t, uint64(1), firstUnsigned[uint64](1))
	require.Equal(t, uintptr(1), firstUnsigned[uintptr](1))
}

func TestInteger(t *testing.T) {
	t.Parallel()
	require.Equal(t, 3, firstInteger[int](3))
	require.Equal(t, uint64(3), firstInteger[uint64](3))
}

func TestFloat(t *testing.T) {
	t.Parallel()
	require.Equal(t, float32(1.5), firstFloat[float32](1.5))
	require.Equal(t, 1.5, firstFloat[float64](1.5))
}

func TestNumbers(t *testing.T) {
	t.Parallel()
	require.Equal(t, 5, sum[int](2, 3))
	require.Equal(t, 5.75, sum[float64](2.5, 3.25))
}

func TestNumbersString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a", firstNumbersString[string]("a", "b"))
	require.Equal(t, 1, firstNumbersString[int](1))
	require.Equal(t, float32(1.5), firstNumbersString[float32](1.5))
}

func TestPrimitive(t *testing.T) {
	t.Parallel()
	require.Equal(t, 7, identity[int](7))
	require.Equal(t, "hello", identity[string]("hello"))
	require.True(t, identity[bool](true))
	require.Equal(t, 1.5, identity[float64](1.5))
}

// Named underlying types must also satisfy the constraints via the
// tilde (~) operator in the constraint definitions.
type namedInt int
type namedUint uint
type namedFloat float64
type namedString string

func TestNamedUnderlyingTypes(t *testing.T) {
	t.Parallel()
	require.Equal(t, namedInt(1), firstSigned[namedInt](1))
	require.Equal(t, namedUint(1), firstUnsigned[namedUint](1))
	require.Equal(t, namedFloat(1.5), firstFloat[namedFloat](1.5))
	require.Equal(t, namedString("x"), identity[namedString]("x"))
}
