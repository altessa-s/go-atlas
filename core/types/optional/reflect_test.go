// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

// TestOptionalFieldLayout guards the field order and names that the unsafe
// reflect helpers (readUnexportedField/writeUnexportedField) rely on. Those
// helpers alias Optional[T] storage by field index; if a future refactor
// reorders or renames the fields, this test fails loudly instead of letting
// the unsafe access silently corrupt values.
func TestOptionalFieldLayout(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[optional.Optional[int]]()
	require.Equal(t, 2, typ.NumField(), "Optional[T] must have exactly two fields")

	require.Equal(t, "value", typ.Field(0).Name, "field 0 must be 'value'")
	require.Equal(t, "present", typ.Field(1).Name, "field 1 must be 'present'")
	require.Equal(t, reflect.Bool, typ.Field(1).Type.Kind(), "field 'present' must be bool")
}

func TestIsOptionalType(t *testing.T) {
	t.Parallel()

	require.True(t, optional.IsOptionalType(reflect.TypeFor[optional.Optional[int]]()))
	require.True(t, optional.IsOptionalType(reflect.TypeFor[optional.Optional[string]]()))
	require.True(t, optional.IsOptionalType(reflect.TypeFor[optional.Optional[time.Time]]()))
	require.True(t, optional.IsOptionalType(reflect.TypeFor[optional.Optional[*int]]()))

	require.False(t, optional.IsOptionalType(nil))
	require.False(t, optional.IsOptionalType(reflect.TypeFor[int]()))
	require.False(t, optional.IsOptionalType(reflect.TypeFor[*optional.Optional[int]]()))
	require.False(t, optional.IsOptionalType(reflect.TypeFor[struct{}]()))
}

func TestInnerType(t *testing.T) {
	t.Parallel()

	require.Equal(t, reflect.TypeFor[int](),
		optional.InnerType(reflect.TypeFor[optional.Optional[int]]()))
	require.Equal(t, reflect.TypeFor[time.Time](),
		optional.InnerType(reflect.TypeFor[optional.Optional[time.Time]]()))

	require.Panics(t, func() {
		optional.InnerType(reflect.TypeFor[int]())
	})
}

func TestSomeReflect_RoundTrip(t *testing.T) {
	t.Parallel()

	optType := reflect.TypeFor[optional.Optional[string]]()
	v := optional.SomeReflect(optType, reflect.ValueOf("hi"))

	got, ok := v.Interface().(optional.Optional[string])
	require.True(t, ok)
	require.True(t, got.IsSome())
	require.Equal(t, "hi", got.Value())
}

func TestNoneReflect_RoundTrip(t *testing.T) {
	t.Parallel()

	optType := reflect.TypeFor[optional.Optional[string]]()
	v := optional.NoneReflect(optType)

	got, ok := v.Interface().(optional.Optional[string])
	require.True(t, ok)
	require.True(t, got.IsNone())
	require.Equal(t, "", got.Value())
}

func TestGetReflect_Some(t *testing.T) {
	t.Parallel()

	opt := optional.Some(42)
	value, present := optional.GetReflect(reflect.ValueOf(opt))
	require.True(t, present)
	require.Equal(t, 42, value.Interface())
}

func TestGetReflect_None(t *testing.T) {
	t.Parallel()

	opt := optional.None[int]()
	value, present := optional.GetReflect(reflect.ValueOf(opt))
	require.False(t, present)
	require.Equal(t, 0, value.Interface())
}

func TestGetReflect_AddressableField(t *testing.T) {
	t.Parallel()

	type doc struct {
		DeletedAt optional.Optional[time.Time]
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	d := doc{DeletedAt: optional.Some(when)}
	field := reflect.ValueOf(&d).Elem().Field(0)

	value, present := optional.GetReflect(field)
	require.True(t, present)
	require.Equal(t, when, value.Interface())
}

func TestSomeReflect_AssignableInterface(t *testing.T) {
	t.Parallel()

	type stringer interface{ String() string }
	optType := reflect.TypeFor[optional.Optional[stringer]]()
	v := optional.SomeReflect(optType, reflect.ValueOf(time.Second))

	got, ok := v.Interface().(optional.Optional[stringer])
	require.True(t, ok)
	inner, ok2 := got.Get()
	require.True(t, ok2)
	require.Equal(t, "1s", inner.String())
}

func TestSomeReflect_PanicsOnNonOptional(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		optional.SomeReflect(reflect.TypeFor[int](), reflect.ValueOf(0))
	})
}

func TestSomeReflect_PanicsOnAssignabilityMismatch(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		optional.SomeReflect(
			reflect.TypeFor[optional.Optional[int]](),
			reflect.ValueOf("string"),
		)
	})
}
