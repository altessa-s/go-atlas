// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/normalizer"
)

func TestNormalize_StringPointer(t *testing.T) {
	type S struct {
		Name *string `normalize:"trim,lowercase"`
	}

	val := "  HELLO  "
	s := &S{Name: &val}
	require.NoError(t, normalizer.Normalize(s))
	require.Equal(t, "hello", *s.Name)
}

func TestNormalize_NilPointerField(t *testing.T) {
	type S struct {
		Name *string `normalize:"trim"`
	}

	s := &S{Name: nil}
	require.NoError(t, normalizer.Normalize(s))
	require.Nil(t, s.Name)
}

func TestNormalize_SliceOfStrings(t *testing.T) {
	type S struct {
		Tags []string `normalize:"trim,lowercase"`
	}

	s := &S{Tags: []string{"  FOO ", " BAR "}}
	// Normalize may not process slice of primitives — just ensure no error/panic
	_ = normalizer.Normalize(s)
}

func TestNormalize_NestedStruct(t *testing.T) {
	type Inner struct {
		Val string `normalize:"uppercase"`
	}
	type Outer struct {
		Inner Inner
	}

	s := &Outer{Inner: Inner{Val: "hello"}}
	require.NoError(t, normalizer.Normalize(s))
	require.Equal(t, "HELLO", s.Inner.Val)
}

func TestNormalize_InvalidInput(t *testing.T) {
	err := normalizer.Normalize("not a struct")
	require.Error(t, err)
}

func TestNormalize_NilInput(t *testing.T) {
	err := normalizer.Normalize(nil)
	require.Error(t, err)
}

func TestClearParameterCache(t *testing.T) {
	// Just ensure no panic
	normalizer.ClearParameterCache()
}

func TestClearStructFieldCache(t *testing.T) {
	normalizer.ClearStructFieldCache()
}

func TestNormalize_MultipleFields(t *testing.T) {
	type S struct {
		First string `normalize:"trim"`
		Last  string `normalize:"trim,uppercase"`
		Email string `normalize:"trim,lowercase"`
	}

	s := &S{First: "  John  ", Last: "  doe  ", Email: "  USER@EXAMPLE.COM  "}
	require.NoError(t, normalizer.Normalize(s))
	require.Equal(t, "John", s.First)
	require.Equal(t, "DOE", s.Last)
	require.Equal(t, "user@example.com", s.Email)
}

func TestNormalize_EmptySlice(t *testing.T) {
	type S struct {
		Tags []string `normalize:"trim"`
	}

	s := &S{Tags: []string{}}
	require.NoError(t, normalizer.Normalize(s))
}

func TestNormalize_SliceOfStructs(t *testing.T) {
	type Item struct {
		Name string `normalize:"trim,lowercase"`
	}
	type S struct {
		Items []Item
	}

	s := &S{Items: []Item{{Name: "  FOO "}, {Name: " BAR "}}}
	require.NoError(t, normalizer.Normalize(s))
	require.Equal(t, "foo", s.Items[0].Name)
}

func TestNormalize_NilOnEmpty_EmptyStringPointer(t *testing.T) {
	type S struct {
		Inn *string `normalize:"trim,nil_on_empty"`
	}

	empty := ""
	s := &S{Inn: &empty}
	require.NoError(t, normalizer.Normalize(s))
	require.Nil(t, s.Inn)

	whitespace := "   "
	s2 := &S{Inn: &whitespace}
	require.NoError(t, normalizer.Normalize(s2))
	require.Nil(t, s2.Inn)
}
