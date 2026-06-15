// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
)

func TestTagConstructorsAndString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tag    etag.Tag
		weak   bool
		value  string
		string string
	}{
		{name: "strong", tag: etag.Strong("abc"), value: "abc", string: `"abc"`},
		{name: "weak", tag: etag.Weak("abc"), weak: true, value: "abc", string: `W/"abc"`},
		{name: "empty strong", tag: etag.Strong(""), value: "", string: `""`},
		{name: "zero", tag: etag.Tag{}, value: "", string: `""`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.value, tc.tag.Value())
			require.Equal(t, tc.weak, tc.tag.IsWeak())
			require.Equal(t, tc.string, tc.tag.String())
		})
	}
}

func TestTagIsZero(t *testing.T) {
	t.Parallel()
	require.True(t, etag.Tag{}.IsZero())
	require.True(t, etag.Strong("").IsZero()) // empty strong is indistinguishable from absent
	require.False(t, etag.Strong("x").IsZero())
	require.False(t, etag.Weak("x").IsZero())
}

func TestFromModTime(t *testing.T) {
	t.Parallel()

	mod := time.Unix(0, 0x1234abcd)
	tag := etag.FromModTime(0x400, mod)

	require.True(t, tag.IsWeak())
	require.Equal(t, "400-1234abcd", tag.Value())
	require.Equal(t, `W/"400-1234abcd"`, tag.String())

	// Round-trips through Parse.
	got, err := etag.Parse(tag.String())
	require.NoError(t, err)
	require.Equal(t, tag, got)
}

func TestMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		a, b        etag.Tag
		strongMatch bool
		weakMatch   bool
	}{
		{name: "strong vs strong equal", a: etag.Strong("x"), b: etag.Strong("x"), strongMatch: true, weakMatch: true},
		{name: "strong vs strong differ", a: etag.Strong("x"), b: etag.Strong("y")},
		{name: "weak vs weak equal", a: etag.Weak("x"), b: etag.Weak("x"), weakMatch: true},
		{name: "weak vs strong equal", a: etag.Weak("x"), b: etag.Strong("x"), weakMatch: true},
		{name: "zero vs zero", a: etag.Tag{}, b: etag.Tag{}},
		{name: "empty vs empty", a: etag.Strong(""), b: etag.Strong("")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.strongMatch, tc.a.StrongMatch(tc.b))
			require.Equal(t, tc.strongMatch, tc.b.StrongMatch(tc.a), "StrongMatch must be symmetric")
			require.Equal(t, tc.weakMatch, tc.a.WeakMatch(tc.b))
			require.Equal(t, tc.weakMatch, tc.b.WeakMatch(tc.a), "WeakMatch must be symmetric")
		})
	}
}

func TestAIP154Representation(t *testing.T) {
	t.Parallel()

	// AIP-154 stores the RFC 7232 wire form (quotes included) in a resource
	// `etag` string field; base64 digests are common and must round-trip.
	for _, v := range []string{"1a2b3c", "Zm9vYmFy", "a+b/c="} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			field := etag.Strong(v).String() // value placed in the proto etag field
			require.Equal(t, `"`+v+`"`, field)

			got, err := etag.Parse(field) // read back from the field
			require.NoError(t, err)
			require.Equal(t, etag.Strong(v), got)

			weakField := etag.Weak(v).String()
			require.Equal(t, `W/"`+v+`"`, weakField)
			gotWeak, err := etag.Parse(weakField)
			require.NoError(t, err)
			require.Equal(t, etag.Weak(v), gotWeak)
		})
	}

	// AIP-154 mutation validation compares the request etag with a strong match.
	cur := etag.Strong("v1")
	require.True(t, cur.StrongMatch(etag.Strong("v1")))
	require.False(t, cur.StrongMatch(etag.Strong("v2")))
	require.False(t, cur.StrongMatch(etag.Weak("v1")), "weak request etag must not satisfy strong validation")
}
