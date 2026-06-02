// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
)

func TestParser_Parse_Empty(t *testing.T) {
	t.Parallel()
	cases := []string{"", " ", "\t", "\n\n  \t"}
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ob, err := p.Parse(t.Context(), in)
			require.NoError(t, err)
			require.True(t, ob.IsEmpty())
			require.Empty(t, ob.Keys)
		})
	}
}

func TestParser_Parse_SingleKey(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	tests := []struct {
		expr string
		want orderby.Key
	}{
		{"slug", orderby.Key{Name: "slug", Direction: orderby.DirectionAscending}},
		{"slug asc", orderby.Key{Name: "slug", Direction: orderby.DirectionAscending}},
		{"slug desc", orderby.Key{Name: "slug", Direction: orderby.DirectionDescending}},
		{"  slug   desc  ", orderby.Key{Name: "slug", Direction: orderby.DirectionDescending}},
		{"slug DESC", orderby.Key{Name: "slug", Direction: orderby.DirectionDescending}},
		{"slug Asc", orderby.Key{Name: "slug", Direction: orderby.DirectionAscending}},
		{
			"address.city desc",
			orderby.Key{Name: "address.city", Direction: orderby.DirectionDescending},
		},
		{
			"_internal9 asc",
			orderby.Key{Name: "_internal9", Direction: orderby.DirectionAscending},
		},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			ob, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err)
			require.Len(t, ob.Keys, 1)
			require.Equal(t, tt.want, ob.Keys[0])
		})
	}
}

func TestParser_Parse_MultiKey(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	ob, err := p.Parse(t.Context(), "create_time desc, slug, address.city asc")
	require.NoError(t, err)
	require.Len(t, ob.Keys, 3)
	require.Equal(t, "create_time", ob.Keys[0].Name)
	require.Equal(t, orderby.DirectionDescending, ob.Keys[0].Direction)
	require.Equal(t, "slug", ob.Keys[1].Name)
	require.Equal(t, orderby.DirectionAscending, ob.Keys[1].Direction)
	require.Equal(t, "address.city", ob.Keys[2].Name)
	require.Equal(t, orderby.DirectionAscending, ob.Keys[2].Direction)
}

func TestParser_Parse_WhitespaceVariants(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	inputs := []string{
		"a,b desc,c",
		"a , b desc , c",
		"  a   ,\tb\tdesc , c  ",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ob, err := p.Parse(t.Context(), in)
			require.NoError(t, err)
			require.Len(t, ob.Keys, 3)
			require.Equal(t, "a", ob.Keys[0].Name)
			require.Equal(t, orderby.DirectionAscending, ob.Keys[0].Direction)
			require.Equal(t, "b", ob.Keys[1].Name)
			require.Equal(t, orderby.DirectionDescending, ob.Keys[1].Direction)
			require.Equal(t, "c", ob.Keys[2].Name)
		})
	}
}

func TestParser_Parse_ErrEmptyClause(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	cases := []string{",a", "a,", "a,,b", ",", ", ,"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), in)
			require.ErrorIs(t, err, orderby.ErrEmptyClause)
		})
	}
}

func TestParser_Parse_ErrParseFailed_TooManyTokens(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "field asc extra")
	require.ErrorIs(t, err, orderby.ErrParseFailed)
}

func TestParser_Parse_ErrInvalidDirection(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "field upward")
	require.ErrorIs(t, err, orderby.ErrInvalidDirection)
}

func TestParser_Parse_CaseSensitiveDirection(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache(), orderby.WithCaseSensitiveDirection())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "field DESC")
	require.ErrorIs(t, err, orderby.ErrInvalidDirection)

	ob, err := p.Parse(t.Context(), "field desc")
	require.NoError(t, err)
	require.Equal(t, orderby.DirectionDescending, ob.Keys[0].Direction)
}

func TestParser_Parse_ErrInvalidFieldPath(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	cases := []string{
		"1foo",        // leading digit
		"foo-bar",     // hyphen disallowed
		"foo..bar",    // consecutive dots → empty segment
		".foo",        // leading dot
		"foo.",        // trailing dot
		"foo bar baz", // would be three tokens; handled by ErrParseFailed
		"foo$",        // special char
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), in)
			require.Error(t, err)
			// Either invalid field path or parse failed (too many tokens).
			ok := errors.Is(err, orderby.ErrInvalidFieldPath) ||
				errors.Is(err, orderby.ErrParseFailed)
			require.True(t, ok, "unexpected error %v", err)
		})
	}
}

// TestParser_Parse_ArrayIndexPaths_Default covers the AIP-132-strict
// default: an all-digit segment fails grammar validation.
func TestParser_Parse_ArrayIndexPaths_Default(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	cases := []string{
		"tags.0",
		"items.5.name",
		"a.0.b.1",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), in)
			require.ErrorIs(t, err, orderby.ErrInvalidFieldPath)
		})
	}
}

// TestParser_Parse_ArrayIndexPaths_Enabled exercises the opt-in
// relaxation: with WithAllowArrayIndexPaths, all-digit segments are
// accepted alongside regular identifiers.
func TestParser_Parse_ArrayIndexPaths_Enabled(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithAllowArrayIndexPaths(),
	)
	require.NoError(t, err)

	tests := []struct {
		expr string
		name string
	}{
		{"tags.0", "tags.0"},
		{"items.5.name desc", "items.5.name"},
		{"a.001.b", "a.001.b"},
		{"a.0.b.1", "a.0.b.1"},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			ob, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err)
			require.Len(t, ob.Keys, 1)
			require.Equal(t, tt.name, ob.Keys[0].Name)
		})
	}

	// Negative cases stay rejected even with the option on — pure
	// alphanumeric mixing is still invalid (no "1foo", no "foo-bar").
	_, err = p.Parse(t.Context(), "1foo")
	require.ErrorIs(t, err, orderby.ErrInvalidFieldPath)
	_, err = p.Parse(t.Context(), "foo-bar")
	require.ErrorIs(t, err, orderby.ErrInvalidFieldPath)
}

func TestParser_Parse_LimitMaxExpressionLength(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithMaxExpressionLength(10),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "abcdefghijk") // 11 bytes
	require.ErrorIs(t, err, orderby.ErrExpressionTooLong)
}

func TestParser_Parse_LimitMaxKeys(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithMaxKeys(2),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "a, b, c")
	require.ErrorIs(t, err, orderby.ErrMaxKeysExceeded)

	ob, err := p.Parse(t.Context(), "a, b")
	require.NoError(t, err)
	require.Len(t, ob.Keys, 2)
}

// TestParser_Parse_MaxKeysPrecedesEmptyClause locks in the documented
// error precedence: an input that exceeds maxKeys *and* contains an
// empty clause must surface ErrMaxKeysExceeded, not ErrEmptyClause,
// regardless of where the empty clause sits in the input.
func TestParser_Parse_MaxKeysPrecedesEmptyClause(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithMaxKeys(2),
	)
	require.NoError(t, err)

	cases := []string{
		"a,,b,c", // empty clause in the middle
		",a,b,c", // empty clause at the start
		"a,b,c,", // empty clause at the end
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), in)
			require.ErrorIs(t, err, orderby.ErrMaxKeysExceeded)
		})
	}
}

func TestParser_Parse_LimitMaxFieldPathDepth(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithMaxFieldPathDepth(2),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "a.b.c")
	require.ErrorIs(t, err, orderby.ErrMaxFieldPathDepthExceeded)

	ob, err := p.Parse(t.Context(), "a.b desc")
	require.NoError(t, err)
	require.Equal(t, "a.b", ob.Keys[0].Name)
}

func TestParser_Parse_LimitMaxFieldNameLength(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(
		orderby.WithParserNoCache(),
		orderby.WithMaxFieldNameLength(5),
	)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), strings.Repeat("a", 6))
	require.ErrorIs(t, err, orderby.ErrMaxFieldNameLengthExceeded)

	_, err = p.Parse(t.Context(), strings.Repeat("a", 5))
	require.NoError(t, err)
}

func TestParser_Parse_ErrDuplicateKey(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	cases := []string{
		"slug, slug",
		"slug, slug desc",
		"slug desc, slug",
		"slug, name, slug",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), in)
			require.ErrorIs(t, err, orderby.ErrDuplicateKey)
		})
	}

	// Case-sensitive comparison: "Slug" and "slug" are distinct
	// identifiers under AIP-132, no duplicate.
	ob, err := p.Parse(t.Context(), "Slug, slug")
	require.NoError(t, err)
	require.Len(t, ob.Keys, 2)
}

// TestParser_Parse_DuplicateAfterSyntaxError locks the precedence rule:
// a malformed duplicate clause must surface its syntactic problem before
// the duplicate-key check fires.
func TestParser_Parse_DuplicateAfterSyntaxError(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), "slug, slug upward")
	require.ErrorIs(t, err, orderby.ErrInvalidDirection)
	require.NotErrorIs(t, err, orderby.ErrDuplicateKey)
}

func TestParser_Parse_Cache(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserCacheSize(8))
	require.NoError(t, err)

	const expr = "create_time desc, slug"
	a, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	b, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	require.Equal(t, a, b)
}

// TestParser_Parse_CacheNoAliasing guards against the cache handing out
// the same Spec.Keys backing array to multiple callers. Mutating one
// Parse result must not corrupt a subsequent cache hit.
func TestParser_Parse_CacheNoAliasing(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserCacheSize(8))
	require.NoError(t, err)

	const expr = "address.city desc, slug"

	first, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	require.Equal(t, "address.city", first.Keys[0].Name)

	// Mutate everything the caller could plausibly touch.
	first.Keys[0].Name = "hijacked"
	first.Keys[0].Direction = orderby.DirectionAscending
	first.Keys = append(first.Keys, orderby.Key{Name: "extra"})

	second, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	require.Len(t, second.Keys, 2)
	require.Equal(t, "address.city", second.Keys[0].Name)
	require.Equal(t, orderby.DirectionDescending, second.Keys[0].Direction)
	require.Equal(t, "slug", second.Keys[1].Name)
}

func TestParser_MustParse(t *testing.T) {
	t.Parallel()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(t, err)

	ob := p.MustParse("create_time desc")
	require.Equal(t, "create_time", ob.Keys[0].Name)

	require.Panics(t, func() { _ = p.MustParse("foo bar baz") })
}

func TestDirection_String(t *testing.T) {
	t.Parallel()
	require.Equal(t, "asc", orderby.DirectionAscending.String())
	require.Equal(t, "desc", orderby.DirectionDescending.String())
	// Unknown values fall back to a Stringer-style placeholder so
	// fmt-formatted output never silently swallows an invalid Direction.
	require.Equal(t, "Direction(7)", orderby.Direction(7).String())
}
