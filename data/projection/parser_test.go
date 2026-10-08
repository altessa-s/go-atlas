// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection_test

import (
	"cmp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/projection"
)

func newParser(tb testing.TB, opts ...projection.ParserOption) *projection.Parser {
	tb.Helper()
	p, err := projection.NewParser(opts...)
	require.NoError(tb, err)
	return p
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr error
	}{
		{name: "empty selects all", in: ""},
		{name: "whitespace selects all", in: "   \t"},
		{name: "star selects all", in: " * "},
		{name: "single", in: "name", want: []string{"name"}},
		{name: "sorted and trimmed", in: " title , author.name,id ", want: []string{"author.name", "id", "title"}},
		{name: "duplicates dropped", in: "a,b,a", want: []string{"a", "b"}},
		{name: "parent and child kept", in: "address.city,address", want: []string{"address", "address.city"}},
		{name: "underscore and digits", in: "_x1.y_2", want: []string{"_x1.y_2"}},
		{name: "empty clause", in: "a,,b", wantErr: projection.ErrEmptyClause},
		{name: "trailing comma", in: "a,", wantErr: projection.ErrEmptyClause},
		{name: "leading digit", in: "1a", wantErr: projection.ErrInvalidFieldPath},
		{name: "double dot", in: "a..b", wantErr: projection.ErrInvalidFieldPath},
		{name: "inner space", in: "a b", wantErr: projection.ErrInvalidFieldPath},
		{name: "dollar operator", in: "$where", wantErr: projection.ErrInvalidFieldPath},
		{name: "dash", in: "a-b", wantErr: projection.ErrInvalidFieldPath},
		{name: "wildcard segment", in: "items.*.name", wantErr: projection.ErrUnsupportedPath},
		{name: "star mixed with paths", in: "a,*", wantErr: projection.ErrUnsupportedPath},
		{name: "backtick key", in: "labels.`a b`", wantErr: projection.ErrUnsupportedPath},
		{name: "numeric segment", in: "items.0", wantErr: projection.ErrUnsupportedPath},
		{name: "too deep", in: "a.b.c.d.e.f.g.h.i", wantErr: projection.ErrMaxFieldPathDepthExceeded},
		{name: "too long name", in: strings.Repeat("a", 129), wantErr: projection.ErrMaxFieldNameLengthExceeded},
	}
	p := newParser(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec, err := p.Parse(t.Context(), tc.in)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, spec.Paths)
		})
	}
}

func TestParseLimits(t *testing.T) {
	t.Parallel()

	p := newParser(t, projection.WithMaxPaths(2), projection.WithMaxExpressionLength(16))

	_, err := p.Parse(t.Context(), "a,a,a")
	require.ErrorIs(t, err, projection.ErrMaxPathsExceeded, "the limit applies before deduplication")

	_, err = p.Parse(t.Context(), strings.Repeat("a", 17))
	require.ErrorIs(t, err, projection.ErrExpressionTooLong)

	_, err = p.Parse(t.Context(), "a,,,")
	require.ErrorIs(t, err, projection.ErrMaxPathsExceeded, "count is checked before clause syntax")
}

func TestParseReturnsIndependentCopy(t *testing.T) {
	t.Parallel()

	p := newParser(t)
	first, err := p.Parse(t.Context(), "a,b")
	require.NoError(t, err)
	first.Paths[0] = "mutated"

	second, err := p.Parse(t.Context(), "a,b")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, second.Paths)
}

func TestParseNoCache(t *testing.T) {
	t.Parallel()

	p := newParser(t, projection.WithParserNoCache())
	spec, err := p.Parse(t.Context(), "b,a")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, spec.Paths)
}

func TestParsePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr error
	}{
		{name: "nil selects all"},
		{name: "star selects all", in: []string{"*"}},
		{name: "paths", in: []string{"title", "author.name"}, want: []string{"author.name", "title"}},
		{name: "comma inside element", in: []string{"a,b"}, wantErr: projection.ErrInvalidFieldPath},
		{name: "empty element", in: []string{""}, wantErr: projection.ErrEmptyClause},
		{name: "blank element among others", in: []string{"a", " "}, wantErr: projection.ErrEmptyClause},
		{name: "wildcard", in: []string{"items.*"}, wantErr: projection.ErrUnsupportedPath},
	}
	p := newParser(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec, err := p.ParsePaths(t.Context(), tc.in)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, spec.Paths)
		})
	}
}

func TestParsePathsLimits(t *testing.T) {
	t.Parallel()

	p := newParser(t, projection.WithMaxPaths(2), projection.WithMaxExpressionLength(5))

	_, err := p.ParsePaths(t.Context(), []string{"a", "a", "a"})
	require.ErrorIs(t, err, projection.ErrMaxPathsExceeded)

	_, err = p.ParsePaths(t.Context(), []string{"abc", "def"})
	require.ErrorIs(t, err, projection.ErrExpressionTooLong, "joined length counts the separators")
}

func TestMustParse(t *testing.T) {
	t.Parallel()

	p := newParser(t)
	require.Equal(t, []string{"a"}, p.MustParse("a").Paths)
	require.Panics(t, func() { p.MustParse("a,,") })
}

func TestSpec(t *testing.T) {
	t.Parallel()

	var empty projection.Spec
	require.True(t, empty.IsEmpty())
	require.True(t, empty.Contains("anything"))
	require.Equal(t, projection.Spec{}, empty.Clone())

	spec := projection.Spec{Paths: []string{"address", "user.name"}}
	require.True(t, spec.Contains("address"))
	require.True(t, spec.Contains("address.city"))
	require.False(t, spec.Contains("addressee"))
	require.False(t, spec.Contains("user"), "a partially selected parent is not contained")
	require.True(t, spec.Contains("user.name"))

	clone := spec.Clone()
	clone.Paths[0] = "x"
	require.Equal(t, "address", spec.Paths[0])
}

func TestSpecRelative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		paths  []string
		prefix string
		want   []string
		wantOK bool
	}{
		{name: "empty selects all", wantOK: true},
		{name: "prefix itself selects all", paths: []string{"items", "items.name"}, wantOK: true},
		{name: "re-rooted", paths: []string{"items.name", "items.address.city", "next_page_token"}, want: []string{"address.city", "name"}, wantOK: true},
		{name: "boundary is a dot", paths: []string{"itemsx.name"}},
		{name: "nothing under prefix", paths: []string{"next_page_token"}},
		{name: "ancestor selects all", paths: []string{"result"}, prefix: "result.items", wantOK: true},
		{name: "nested prefix", paths: []string{"result.items.name", "result.total"}, prefix: "result.items", want: []string{"name"}, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := projection.Spec{Paths: tc.paths}.Relative(cmp.Or(tc.prefix, "items"))
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got.Paths)
		})
	}
}
