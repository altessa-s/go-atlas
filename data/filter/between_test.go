// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBetween_Expansion(t *testing.T) {
	field := &IdentNode{Name: "age"}
	lo := &LiteralNode{Value: int64(18)}
	hi := &LiteralNode{Value: int64(65)}

	node, err := Between()([]Node{field, lo, hi})
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok, "expected BinaryOpNode, got %T", node)
	require.Equal(t, OpAnd, andNode.Op)

	left, ok := andNode.Left.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpGTE, left.Op)
	require.Same(t, field, left.Left)
	require.Same(t, lo, left.Right)

	right, ok := andNode.Right.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpLTE, right.Op)
	require.Same(t, field, right.Left)
	require.Same(t, hi, right.Right)
}

func TestBetween_WrongArity(t *testing.T) {
	field := &IdentNode{Name: "age"}
	lit := &LiteralNode{Value: int64(1)}

	cases := [][]Node{
		nil,
		{field},
		{field, lit},
		{field, lit, lit, lit},
	}
	for _, args := range cases {
		_, err := Between()(args)
		require.ErrorIs(t, err, ErrInvalidExpression, "args=%v", args)
	}
}

func TestBetween_FirstArgMustBeField(t *testing.T) {
	args := []Node{
		&LiteralNode{Value: "not-a-field"},
		&LiteralNode{Value: int64(1)},
		&LiteralNode{Value: int64(2)},
	}
	_, err := Between()(args)
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "field identifier")
}

func TestBetweenFilter_OneEntry(t *testing.T) {
	got := BetweenFilter()
	require.Len(t, got, 1)
	require.NotNil(t, got[BetweenFunc])
}

func TestBetween_ParserPickup(t *testing.T) {
	withCleanGlobalRegistry(t)
	require.NoError(t, RegisterFunctions(BetweenFilter()))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `between(age, 18, 65) && active`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpAnd, andNode.Op)

	// Left side: the expanded between(age, 18, 65) — itself an And-tree.
	betweenAnd, ok := andNode.Left.(*BinaryOpNode)
	require.True(t, ok, "expected expanded BinaryOpNode on left, got %T", andNode.Left)
	require.Equal(t, OpAnd, betweenAnd.Op)
	require.Equal(t, OpGTE, betweenAnd.Left.(*BinaryOpNode).Op)
	require.Equal(t, "age", betweenAnd.Left.(*BinaryOpNode).Left.(*IdentNode).Name)
	require.Equal(t, OpLTE, betweenAnd.Right.(*BinaryOpNode).Op)
}
