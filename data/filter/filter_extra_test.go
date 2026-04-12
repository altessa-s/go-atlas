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

func TestTranslatorConfig_MaxDepth(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	filter.WithMaxDepth(5)(cfg)
	require.Equal(t, 5, cfg.MaxDepth())
}

func TestTranslatorConfig_StrictMode(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	filter.WithStrictMode(true)(cfg)
	require.True(t, cfg.StrictMode(), "StrictMode() should be true")
}

func TestTranslatorConfig_SetAllowedFields(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	cfg.SetAllowedFields(map[string]struct{}{"name": {}, "age": {}})
	require.True(t, cfg.IsFieldAllowed("name"), "name should be allowed")
	require.False(t, cfg.IsFieldAllowed("other"), "other should not be allowed")
}

func TestTranslatorConfig_SetFieldMapping(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	cfg.SetFieldMapping(map[string]string{"name": "full_name"})
	require.Equal(t, "full_name", cfg.ApplyFieldMapping("name"))
}

func TestEvaluator_NilNotEqual(t *testing.T) {
	parser, _ := filter.NewParser()
	node, _ := parser.Parse(t.Context(), "name != null")
	ev := filter.NewEvaluator()

	result, err := ev.Evaluate(node, map[string]any{"name": "hello"})
	require.NoError(t, err)
	require.True(t, result, "'hello' != null should be true")
}
