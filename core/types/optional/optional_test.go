// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

func TestSome(t *testing.T) {
	t.Parallel()

	o := optional.Some(42)

	v, ok := o.Get()
	require.Equal(t, 42, v)
	require.True(t, ok)
	require.True(t, o.IsSome())
	require.False(t, o.IsNone())
	require.Equal(t, 42, o.Value())
	require.Equal(t, 42, o.OrDefault(99))

	got := o.OrElse(func() int {
		t.Fatal("OrElse must not invoke fn on Some")
		return 0
	})
	require.Equal(t, 42, got)
}

func TestNone(t *testing.T) {
	t.Parallel()

	o := optional.None[int]()

	v, ok := o.Get()
	require.Equal(t, 0, v)
	require.False(t, ok)
	require.False(t, o.IsSome())
	require.True(t, o.IsNone())
	require.Equal(t, 0, o.Value())
	require.Equal(t, 99, o.OrDefault(99))

	var calls int
	got := o.OrElse(func() int {
		calls++
		return 7
	})
	require.Equal(t, 7, got)
	require.Equal(t, 1, calls)
}

func TestSomeZeroIsDistinctFromNone(t *testing.T) {
	t.Parallel()

	some := optional.Some("")
	none := optional.None[string]()

	sv, sok := some.Get()
	require.Equal(t, "", sv)
	require.True(t, sok, "Some(\"\") must report present=true")

	nv, nok := none.Get()
	require.Equal(t, "", nv)
	require.False(t, nok, "None must report present=false")

	require.NotEqual(t, some, none, "Some(zero) and None must not compare equal")
}

func TestZeroValueIsNone(t *testing.T) {
	t.Parallel()

	var o optional.Optional[int]

	v, ok := o.Get()
	require.Equal(t, 0, v)
	require.False(t, ok)
	require.True(t, o.IsNone())
	require.False(t, o.IsSome())
}

func TestOfEquivalences(t *testing.T) {
	t.Parallel()

	t.Run("ok=true equals Some", func(t *testing.T) {
		t.Parallel()

		a := optional.Of(42, true)
		b := optional.Some(42)

		require.Equal(t, b, a)

		av, aok := a.Get()
		bv, bok := b.Get()
		require.Equal(t, bv, av)
		require.Equal(t, bok, aok)
	})

	t.Run("ok=false equals None and discards value", func(t *testing.T) {
		t.Parallel()

		a := optional.Of(42, false)
		b := optional.None[int]()

		require.Equal(t, b, a, "ok=false must collapse to None regardless of v")

		av, aok := a.Get()
		require.Equal(t, 0, av, "Of(v, false) must discard v")
		require.False(t, aok)
	})
}

func TestOrElseInvocationCount(t *testing.T) {
	t.Parallel()

	t.Run("not called on Some", func(t *testing.T) {
		t.Parallel()

		var calls int
		got := optional.Some("x").OrElse(func() string {
			calls++
			return "y"
		})
		require.Equal(t, "x", got)
		require.Zero(t, calls)
	})

	t.Run("called once on None", func(t *testing.T) {
		t.Parallel()

		var calls int
		got := optional.None[string]().OrElse(func() string {
			calls++
			return "y"
		})
		require.Equal(t, "y", got)
		require.Equal(t, 1, calls)
	})
}

type fooStruct struct {
	A int
	B string
}

func TestGenericInstantiation(t *testing.T) {
	t.Parallel()

	t.Run("int", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 5, optional.Some(5).Value())
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "hi", optional.Some("hi").Value())
	})

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		s := fooStruct{A: 1, B: "x"}
		require.Equal(t, s, optional.Some(s).Value())
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()
		p := &fooStruct{A: 2}
		require.Same(t, p, optional.Some(p).Value())
	})

	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		sl := []int{1, 2, 3}
		require.Equal(t, sl, optional.Some(sl).Value())
	})

	t.Run("None of pointer is empty", func(t *testing.T) {
		t.Parallel()
		o := optional.None[*fooStruct]()
		require.True(t, o.IsNone())
		require.Nil(t, o.Value())
	})
}

func TestComparable(t *testing.T) {
	t.Parallel()

	require.Equal(t, optional.Some(1), optional.Some(1))
	require.NotEqual(t, optional.Some(1), optional.Some(2))
	require.Equal(t, optional.None[int](), optional.None[int]())
	require.NotEqual(t, optional.Some(0), optional.None[int]())
}

func TestFromPtr(t *testing.T) {
	t.Parallel()

	t.Run("nil pointer returns None", func(t *testing.T) {
		t.Parallel()
		var ptr *int
		opt := optional.FromPtr(ptr)
		require.True(t, opt.IsNone())
		require.False(t, opt.IsSome())
	})

	t.Run("non-nil pointer returns Some", func(t *testing.T) {
		t.Parallel()
		value := 42
		opt := optional.FromPtr(&value)
		require.True(t, opt.IsSome())
		require.False(t, opt.IsNone())
		v, ok := opt.Get()
		require.True(t, ok)
		require.Equal(t, 42, v)
	})

	t.Run("zero value pointer returns Some(zero)", func(t *testing.T) {
		t.Parallel()
		var zero int
		opt := optional.FromPtr(&zero)
		require.True(t, opt.IsSome())
		v, ok := opt.Get()
		require.True(t, ok)
		require.Equal(t, 0, v)
	})

	t.Run("string type", func(t *testing.T) {
		t.Parallel()
		str := "hello"
		opt := optional.FromPtr(&str)
		require.Equal(t, "hello", opt.Value())

		var nilStr *string
		opt = optional.FromPtr(nilStr)
		require.True(t, opt.IsNone())
	})

	t.Run("struct type", func(t *testing.T) {
		t.Parallel()
		type Person struct {
			Name string
			Age  int
		}

		person := Person{Name: "Alice", Age: 30}
		opt := optional.FromPtr(&person)
		require.Equal(t, Person{Name: "Alice", Age: 30}, opt.Value())

		var nilPerson *Person
		opt = optional.FromPtr(nilPerson)
		require.True(t, opt.IsNone())
	})
}

func TestToPtr(t *testing.T) {
	t.Parallel()

	t.Run("None returns nil", func(t *testing.T) {
		t.Parallel()
		opt := optional.None[int]()
		ptr := optional.ToPtr(opt)
		require.Nil(t, ptr)
	})

	t.Run("Some returns pointer to value", func(t *testing.T) {
		t.Parallel()
		opt := optional.Some(42)
		ptr := optional.ToPtr(opt)
		require.NotNil(t, ptr)
		require.Equal(t, 42, *ptr)
	})

	t.Run("Some(zero) returns pointer to zero", func(t *testing.T) {
		t.Parallel()
		opt := optional.Some(0)
		ptr := optional.ToPtr(opt)
		require.NotNil(t, ptr)
		require.Equal(t, 0, *ptr)
	})

	t.Run("string type", func(t *testing.T) {
		t.Parallel()
		opt := optional.Some("hello")
		ptr := optional.ToPtr(opt)
		require.NotNil(t, ptr)
		require.Equal(t, "hello", *ptr)

		opt = optional.None[string]()
		ptr = optional.ToPtr(opt)
		require.Nil(t, ptr)
	})

	t.Run("struct type", func(t *testing.T) {
		t.Parallel()
		type Person struct {
			Name string
			Age  int
		}

		opt := optional.Some(Person{Name: "Bob", Age: 25})
		ptr := optional.ToPtr(opt)
		require.NotNil(t, ptr)
		require.Equal(t, Person{Name: "Bob", Age: 25}, *ptr)

		opt = optional.None[Person]()
		ptr = optional.ToPtr(opt)
		require.Nil(t, ptr)
	})
}

func TestFromPtrToPtr_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("nil roundtrip", func(t *testing.T) {
		t.Parallel()
		var original *int
		opt := optional.FromPtr(original)
		result := optional.ToPtr(opt)
		require.Nil(t, result)
	})

	t.Run("value roundtrip", func(t *testing.T) {
		t.Parallel()
		value := 42
		original := &value
		opt := optional.FromPtr(original)
		result := optional.ToPtr(opt)
		require.NotNil(t, result)
		require.Equal(t, 42, *result)
		// Note: result is a new pointer, not the same as original
		require.NotSame(t, original, result)
	})

	t.Run("Some to pointer to Optional", func(t *testing.T) {
		t.Parallel()
		opt := optional.Some(100)
		ptr := optional.ToPtr(opt)
		opt2 := optional.FromPtr(ptr)
		require.Equal(t, opt, opt2)
	})
}
