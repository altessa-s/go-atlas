// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestNodeKinds(t *testing.T) {
	tests := []struct {
		node filter.Node
		want filter.NodeKind
	}{
		{&filter.LiteralNode{Value: 42}, filter.NodeKindLiteral},
		{&filter.IdentNode{Name: "x"}, filter.NodeKindIdent},
		{&filter.BinaryOpNode{Op: filter.OpEqual}, filter.NodeKindBinaryOp},
		{&filter.UnaryOpNode{Op: filter.OpNot}, filter.NodeKindUnaryOp},
		{&filter.CallNode{Op: filter.OpContains}, filter.NodeKindCall},
		{&filter.ListNode{}, filter.NodeKindList},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.node.Kind())
	}
}

func TestUnaryOpNode_Children(t *testing.T) {
	child := &filter.LiteralNode{Value: true}
	node := &filter.UnaryOpNode{Op: filter.OpNot, Operand: child}
	count := 0
	for range node.Children() {
		count++
	}
	require.Equal(t, 1, count, "UnaryOpNode children count")
}

func TestCallNode_Children(t *testing.T) {
	target := &filter.IdentNode{Name: "name"}
	arg := &filter.LiteralNode{Value: "hello"}
	node := &filter.CallNode{Op: filter.OpContains, Target: target, Args: []filter.Node{arg}}
	count := 0
	for range node.Children() {
		count++
	}
	require.Equal(t, 2, count, "CallNode children count")
}

func TestTranslatorContext_MaxDepth(t *testing.T) {
	ctx, err := filter.NewTranslatorContext(filter.WithMaxDepth(5))
	require.NoError(t, err)
	require.Equal(t, 5, ctx.MaxDepth())
}

func TestEvaluator_NilNotEqual(t *testing.T) {
	parser, _ := filter.NewParser()
	node, _ := parser.Parse(t.Context(), "name != null")
	ev := mustEvaluator(t)

	result, err := ev.Evaluate(node, map[string]any{"name": "hello"})
	require.NoError(t, err)
	require.True(t, result, "'hello' != null should be true")
}

// Stopping deep in the tree must end the whole traversal: Walk visits nothing
// after its callback returns false, and AllNodes does not yield after the
// consumer breaks, which a range-over-func loop would turn into a panic.
func TestWalk_StopsWhenCallbackReturnsFalse(t *testing.T) {
	t.Parallel()
	node, err := newTestParser(t).Parse(t.Context(), `a == 1 && b == 2`)
	require.NoError(t, err)

	visits := 0
	filter.Walk(node, func(n filter.Node) bool {
		visits++
		_, isIdent := n.(*filter.IdentNode)
		return !isIdent
	})
	require.Equal(t, 3, visits, "&&, ==, a: nothing after the stop")

	require.NotPanics(t, func() {
		for n := range filter.AllNodes(node) {
			if _, isIdent := n.(*filter.IdentNode); isIdent {
				break
			}
		}
	})
}

// Integers compare exactly: through float64, distinct values above 2^53 and
// around the int64/uint64 boundary would collapse into one.
func TestEvaluator_IntegerComparisonsAreExact(t *testing.T) {
	t.Parallel()
	p := newTestParser(t)
	eval := mustEvaluator(t)
	data := map[string]any{
		"big":  int64(9007199254740993),
		"umax": uint64(math.MaxUint64),
		"neg":  int64(-1),
	}

	for expr, want := range map[string]bool{
		`big == 9007199254740992`:         false,
		`big > 9007199254740992`:          true,
		`big in [9007199254740992]`:       false,
		`big in [9007199254740993]`:       true,
		`umax > 9223372036854775807`:      true,
		`umax == 18446744073709551615u`:   true,
		`umax in [18446744073709551614u]`: false,
		`neg < 18446744073709551615u`:     true,
		`neg in [18446744073709551615u]`:  false,
	} {
		node, err := p.Parse(t.Context(), expr)
		require.NoError(t, err, expr)
		got, err := eval.Evaluate(node, data)
		require.NoError(t, err, expr)
		require.Equal(t, want, got, expr)
	}
}

// in compares bytes and lists without panicking: == on two interfaces holding
// the same non-comparable type panics, and the parser produces both.
func TestEvaluator_InWithNonComparableValues(t *testing.T) {
	t.Parallel()
	p := newTestParser(t)
	eval := mustEvaluator(t)

	for expr, want := range map[string]bool{
		`b"x" in [b"x"]`: true,
		`b"x" in [b"y"]`: false,
		`[1] in [[1]]`:   false,
	} {
		node, err := p.Parse(t.Context(), expr)
		require.NoError(t, err, expr)
		require.NotPanics(t, func() {
			got, evalErr := eval.Evaluate(node, map[string]any{})
			require.NoError(t, evalErr, expr)
			require.Equal(t, want, got, expr)
		}, expr)
	}
}

// matches() keeps ErrInvalidRegex for both a pattern over the length cap and a
// pattern that does not compile, now that it skips ValidateRegex.
func TestEvaluator_MatchesInvalidRegex(t *testing.T) {
	t.Parallel()
	p := newTestParser(t)
	eval := mustEvaluator(t, filter.WithMaxRegexLength(8))
	data := map[string]any{"name": "Alice"}

	for _, expr := range []string{`name.matches("[")`, `name.matches("aaaaaaaaaa")`} {
		node, err := p.Parse(t.Context(), expr)
		require.NoError(t, err, expr)
		_, err = eval.Evaluate(node, data)
		require.ErrorIs(t, err, filter.ErrInvalidRegex, expr)
	}
}
