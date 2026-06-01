// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
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
