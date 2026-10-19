// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func newSearchParser(t *testing.T) *filter.Parser {
	t.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache(), filter.WithCustomFunctions(map[string]filter.CustomFunction{
		"search": filter.TextSearch("code", "name", "description"),
	}))
	require.NoError(t, err)
	return p
}

func TestTextSearchExpansion(t *testing.T) {
	t.Parallel()

	node, err := newSearchParser(t).Parse(t.Context(), `search("  a.b ")`)
	require.NoError(t, err)

	// ((code.matches(re) || name.matches(re)) || description.matches(re)), re = (?i) + QuoteMeta(trimmed text).
	or, ok := node.(*filter.BinaryOpNode)
	require.True(t, ok, "got %T", node)
	require.Equal(t, filter.OpOr, or.Op)
	inner, ok := or.Left.(*filter.BinaryOpNode)
	require.True(t, ok)
	var fields []string
	for _, n := range []filter.Node{inner.Left, inner.Right, or.Right} {
		call, ok := n.(*filter.CallNode)
		require.True(t, ok, "got %T", n)
		require.Equal(t, filter.OpMatches, call.Op)
		ident, ok := call.Target.(*filter.IdentNode)
		require.True(t, ok)
		fields = append(fields, ident.Name)
		require.Len(t, call.Args, 1)
		lit, ok := call.Args[0].(*filter.LiteralNode)
		require.True(t, ok)
		require.Equal(t, `(?i)a\.b`, lit.Value)
	}
	require.Equal(t, []string{"code", "name", "description"}, fields)
}

func TestTextSearchSingleField(t *testing.T) {
	t.Parallel()

	out, err := filter.TextSearch("name")([]filter.Node{&filter.LiteralNode{Value: "x"}})
	require.NoError(t, err)
	call, ok := out.(*filter.CallNode)
	require.True(t, ok, "got %T", out)
	require.Equal(t, filter.OpMatches, call.Op)
}

// TestTextSearchSemantics evaluates the expansion: case-insensitive, literal (no wildcards, no anchors) and over every field.
func TestTextSearchSemantics(t *testing.T) {
	t.Parallel()

	p := newSearchParser(t)
	ev, err := filter.NewEvaluator()
	require.NoError(t, err)
	row := map[string]any{"code": "dev_jun", "name": "Νέος προγραμματιστής", "description": "γράφει κώδικα (C++)"}

	cases := []struct {
		expr string
		want bool
	}{
		{`search("ΠΡΟΓΡΑΜΜΑΤΙ")`, true},
		{`search("DEV_")`, true},
		{`search("c++")`, true},
		{`search("dev.*")`, false},
		{`search("^dev_jun$")`, false},
		{`!search("δοκιμή")`, true},
		{`search("δοκιμή") || code == "dev_jun"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()
			node, err := p.Parse(t.Context(), tc.expr)
			require.NoError(t, err)
			got, err := ev.Evaluate(node, row)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTextSearchRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	p := newSearchParser(t)
	for _, expr := range []string{`search()`, `search("a", "b")`, `search(name)`, `search(1)`, `search(" ")`, `search("")`} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			_, err := p.Parse(t.Context(), expr)
			require.ErrorIs(t, err, filter.ErrInvalidExpression)
		})
	}
}

func TestTextSearchNeedsFields(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() { filter.TextSearch() })
}
