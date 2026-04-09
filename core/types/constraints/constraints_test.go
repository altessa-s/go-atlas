// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package constraints_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// The constraints package only declares generic type sets. There is no
// runtime behaviour to assert, so the tests below exercise each
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
	if got := firstSigned[int](1, 2); got != 1 {
		t.Fatalf("int: got %d, want 1", got)
	}
	if got := firstSigned[int8](1); got != 1 {
		t.Fatalf("int8: got %d, want 1", got)
	}
	if got := firstSigned[int16](1); got != 1 {
		t.Fatalf("int16: got %d, want 1", got)
	}
	if got := firstSigned[int32](1); got != 1 {
		t.Fatalf("int32: got %d, want 1", got)
	}
	if got := firstSigned[int64](1); got != 1 {
		t.Fatalf("int64: got %d, want 1", got)
	}
}

func TestUnsigned(t *testing.T) {
	t.Parallel()
	if got := firstUnsigned[uint](1); got != 1 {
		t.Fatalf("uint: got %d, want 1", got)
	}
	if got := firstUnsigned[uint8](1); got != 1 {
		t.Fatalf("uint8: got %d, want 1", got)
	}
	if got := firstUnsigned[uint16](1); got != 1 {
		t.Fatalf("uint16: got %d, want 1", got)
	}
	if got := firstUnsigned[uint32](1); got != 1 {
		t.Fatalf("uint32: got %d, want 1", got)
	}
	if got := firstUnsigned[uint64](1); got != 1 {
		t.Fatalf("uint64: got %d, want 1", got)
	}
	if got := firstUnsigned[uintptr](1); got != 1 {
		t.Fatalf("uintptr: got %d, want 1", got)
	}
}

func TestInteger(t *testing.T) {
	t.Parallel()
	if got := firstInteger[int](3); got != 3 {
		t.Fatalf("int: got %d, want 3", got)
	}
	if got := firstInteger[uint64](3); got != 3 {
		t.Fatalf("uint64: got %d, want 3", got)
	}
}

func TestFloat(t *testing.T) {
	t.Parallel()
	if got := firstFloat[float32](1.5); got != 1.5 {
		t.Fatalf("float32: got %f, want 1.5", got)
	}
	if got := firstFloat[float64](1.5); got != 1.5 {
		t.Fatalf("float64: got %f, want 1.5", got)
	}
}

func TestNumbers(t *testing.T) {
	t.Parallel()
	if got := sum[int](2, 3); got != 5 {
		t.Fatalf("int sum: got %d, want 5", got)
	}
	if got := sum[float64](2.5, 3.25); got != 5.75 {
		t.Fatalf("float64 sum: got %f, want 5.75", got)
	}
}

func TestNumbersString(t *testing.T) {
	t.Parallel()
	if got := firstNumbersString[string]("a", "b"); got != "a" {
		t.Fatalf("string: got %q, want a", got)
	}
	if got := firstNumbersString[int](1); got != 1 {
		t.Fatalf("int: got %d, want 1", got)
	}
	if got := firstNumbersString[float32](1.5); got != 1.5 {
		t.Fatalf("float32: got %f, want 1.5", got)
	}
}

func TestPrimitive(t *testing.T) {
	t.Parallel()
	if got := identity[int](7); got != 7 {
		t.Fatalf("int: got %d, want 7", got)
	}
	if got := identity[string]("hello"); got != "hello" {
		t.Fatalf("string: got %q, want hello", got)
	}
	if got := identity[bool](true); !got {
		t.Fatalf("bool: got false, want true")
	}
	if got := identity[float64](1.5); got != 1.5 {
		t.Fatalf("float64: got %f, want 1.5", got)
	}
}

// Named underlying types must also satisfy the constraints via the
// tilde (~) operator in the constraint definitions.
type namedInt int
type namedUint uint
type namedFloat float64
type namedString string

func TestNamedUnderlyingTypes(t *testing.T) {
	t.Parallel()
	if got := firstSigned[namedInt](1); got != 1 {
		t.Fatalf("namedInt: got %d, want 1", got)
	}
	if got := firstUnsigned[namedUint](1); got != 1 {
		t.Fatalf("namedUint: got %d, want 1", got)
	}
	if got := firstFloat[namedFloat](1.5); got != 1.5 {
		t.Fatalf("namedFloat: got %f, want 1.5", got)
	}
	if got := identity[namedString]("x"); got != "x" {
		t.Fatalf("namedString: got %q, want x", got)
	}
}
