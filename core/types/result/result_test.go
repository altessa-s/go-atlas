// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package result_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/result"
)

var errSentinel = errors.New("result_test: sentinel")

func TestOk(t *testing.T) {
	t.Parallel()

	r := result.Ok(42)

	v, err := r.Get()
	require.Equal(t, 42, v)
	require.NoError(t, err)
	require.True(t, r.IsOk())
	require.False(t, r.IsErr())
	require.Equal(t, 42, r.Value())
	require.NoError(t, r.Err())
	require.Equal(t, 42, r.OrDefault(99))

	got := r.OrElse(func(error) int {
		t.Fatal("OrElse must not invoke fn on Ok")
		return 0
	})
	require.Equal(t, 42, got)
}

func TestErr(t *testing.T) {
	t.Parallel()

	r := result.Err[int](errSentinel)

	v, err := r.Get()
	require.Equal(t, 0, v)
	require.ErrorIs(t, err, errSentinel)
	require.False(t, r.IsOk())
	require.True(t, r.IsErr())
	require.Equal(t, 0, r.Value())
	require.ErrorIs(t, r.Err(), errSentinel)
	require.Equal(t, 99, r.OrDefault(99))

	var seen error
	got := r.OrElse(func(e error) int {
		seen = e
		return 7
	})
	require.Equal(t, 7, got)
	require.ErrorIs(t, seen, errSentinel)
}

func TestOfEquivalences(t *testing.T) {
	t.Parallel()

	t.Run("nil error equals Ok", func(t *testing.T) {
		t.Parallel()

		a := result.Of(42, error(nil))
		b := result.Ok(42)

		av, ae := a.Get()
		bv, be := b.Get()
		require.Equal(t, bv, av)
		require.Equal(t, be, ae)
		require.True(t, a.IsOk())
	})

	t.Run("non-nil error equals Err", func(t *testing.T) {
		t.Parallel()

		a := result.Of(0, errSentinel)
		b := result.Err[int](errSentinel)

		av, ae := a.Get()
		bv, be := b.Get()
		require.Equal(t, bv, av)
		require.ErrorIs(t, ae, be)
		require.True(t, a.IsErr())
	})
}

func TestZeroValueIsOk(t *testing.T) {
	t.Parallel()

	var r result.Result[int]

	v, err := r.Get()
	require.Equal(t, 0, v)
	require.NoError(t, err)
	require.True(t, r.IsOk())
	require.False(t, r.IsErr())
}

func TestErrNilIsIndistinguishableFromOk(t *testing.T) {
	t.Parallel()

	r := result.Err[int](nil)

	v, err := r.Get()
	require.Equal(t, 0, v)
	require.NoError(t, err)
	require.True(t, r.IsOk())
}

func TestErrUnwrapsThroughErrorsIs(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("op: %w", errSentinel)
	r := result.Err[string](wrapped)

	require.ErrorIs(t, r.Err(), errSentinel)

	_, err := r.Get()
	require.ErrorIs(t, err, errSentinel)
}

func TestOrElseInvocationCount(t *testing.T) {
	t.Parallel()

	t.Run("not called on Ok", func(t *testing.T) {
		t.Parallel()

		var calls int
		got := result.Ok("x").OrElse(func(error) string {
			calls++
			return "y"
		})
		require.Equal(t, "x", got)
		require.Zero(t, calls)
	})

	t.Run("called once on Err", func(t *testing.T) {
		t.Parallel()

		var calls int
		got := result.Err[string](errSentinel).OrElse(func(error) string {
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
		require.Equal(t, 5, result.Ok(5).Value())
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "hi", result.Ok("hi").Value())
	})

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		s := fooStruct{A: 1, B: "x"}
		require.Equal(t, s, result.Ok(s).Value())
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()
		p := &fooStruct{A: 2}
		require.Same(t, p, result.Ok(p).Value())
	})

	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		sl := []int{1, 2, 3}
		require.Equal(t, sl, result.Ok(sl).Value())
	})

	t.Run("nil pointer Err", func(t *testing.T) {
		t.Parallel()
		r := result.Err[*fooStruct](errSentinel)
		require.True(t, r.IsErr())
		require.Nil(t, r.Value())
	})
}
