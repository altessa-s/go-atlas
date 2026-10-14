// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestParserResultOwnership(t *testing.T) {
	t.Parallel()
	p, err := filter.NewParser()
	require.NoError(t, err)
	for _, expression := range []string{"role == 'user'", "name.matches('x')", "!(role in ['user', 'guest'])", "blob == b'abc'"} {
		t.Run(expression, func(t *testing.T) {
			t.Parallel()
			node, err := p.Parse(t.Context(), expression)
			require.NoError(t, err)
			filter.Walk(node, func(node filter.Node) bool {
				switch n := node.(type) {
				case *filter.LiteralNode:
					if bytes, ok := n.Value.([]byte); ok {
						bytes[0] = 'X'
					} else {
						n.Value = "mutated"
					}
				case *filter.IdentNode:
					n.Name = "mutated"
				}
				return true
			})
			got, err := p.Parse(t.Context(), expression)
			require.NoError(t, err)
			fresh, err := filter.NewParser(filter.WithParserNoCache())
			require.NoError(t, err)
			want, err := fresh.Parse(t.Context(), expression)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

// Serial: verifies counters of the process-wide regex cache.
func TestRegexCacheAdmitsNewWorkingSet(t *testing.T) {
	e, err := filter.NewEvaluator()
	require.NoError(t, err)
	var last filter.Node
	for i := range 512 {
		last = &filter.CallNode{Op: filter.OpMatches, Target: &filter.IdentNode{Name: "name"},
			Args: []filter.Node{&filter.LiteralNode{Value: fmt.Sprintf("^cache-isolation-%d$", i)}}}
		_, err := e.Evaluate(last, map[string]any{"name": "value"})
		require.NoError(t, err)
	}
	require.LessOrEqual(t, filter.RegexCacheStatsSnapshot().Size, int64(256))
	filter.ResetRegexCacheStats()
	_, err = e.Evaluate(last, map[string]any{"name": "value"})
	require.NoError(t, err)
	require.Equal(t, uint64(1), filter.RegexCacheStatsSnapshot().Hits)
	require.Zero(t, filter.RegexCacheStatsSnapshot().Misses)
}

func TestCustomFunctionSnapshot(t *testing.T) {
	t.Parallel()
	literal := &filter.LiteralNode{Value: []byte("original")}
	p, err := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
		"blob": func([]filter.Node) (filter.Node, error) { return literal, nil },
	}))
	require.NoError(t, err)
	first, err := p.Parse(t.Context(), "blob()")
	require.NoError(t, err)
	literal.Value = []byte("changed")
	second, err := p.Parse(t.Context(), "blob()")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NotSame(t, first, second)
}

func TestCustomMutableLiteralRejected(t *testing.T) {
	t.Parallel()
	p, err := filter.NewParser(filter.WithCustomFunctions(map[string]filter.CustomFunction{
		"mutable": func([]filter.Node) (filter.Node, error) {
			return &filter.LiteralNode{Value: map[string]string{"key": "value"}}, nil
		},
	}))
	require.NoError(t, err)
	_, err = p.Parse(t.Context(), "mutable()")
	require.ErrorIs(t, err, filter.ErrUnsupportedType)
}

func TestParserShared_ReturnsSameNode(t *testing.T) {
	t.Parallel()
	p, err := filter.NewParser()
	require.NoError(t, err)

	expr := `name == "John" && age >= 18`
	first, err := p.ParseShared(t.Context(), expr)
	require.NoError(t, err)

	second, err := p.ParseShared(t.Context(), expr)
	require.NoError(t, err)
	require.Same(t, first, second, "ParseShared must return the cached node directly without cloning")

	owned, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	require.Equal(t, first, owned)
	require.NotSame(t, first, owned, "Parse must return a cloned, caller-owned node")

	// MustParseShared
	mustShared := p.MustParseShared(expr)
	require.Same(t, first, mustShared)

	// Error handling
	_, err = p.ParseShared(t.Context(), "")
	require.ErrorIs(t, err, filter.ErrEmptyExpression)

	require.Panics(t, func() {
		p.MustParseShared("")
	})
}

