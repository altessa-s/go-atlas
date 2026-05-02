// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstant_Expansion(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		op        Operator
		value     any
		wantOp    Operator
		wantField string
		wantValue any
	}{
		{"string equal", "status", OpEqual, "active", OpEqual, "status", "active"},
		{"int gt", "failures", OpGT, int64(0), OpGT, "failures", int64(0)},
		{"float lte", "score", OpLTE, 0.95, OpLTE, "score", 0.95},
		{"bool ne", "active", OpNotEqual, true, OpNotEqual, "active", true},
		{"null eq", "deletedAt", OpEqual, nil, OpEqual, "deletedAt", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := Constant(tt.field, tt.op, tt.value)(nil)
			require.NoError(t, err)

			bin, ok := node.(*BinaryOpNode)
			require.True(t, ok, "expected BinaryOpNode, got %T", node)
			require.Equal(t, tt.wantOp, bin.Op)
			require.Equal(t, tt.wantField, bin.Left.(*IdentNode).Name)
			require.Equal(t, tt.wantValue, bin.Right.(*LiteralNode).Value)
		})
	}
}

func TestConstant_RejectsArgs(t *testing.T) {
	h := Constant("status", OpEqual, "active")
	_, err := h([]Node{&LiteralNode{Value: "x"}})
	require.ErrorIs(t, err, ErrInvalidExpression)
	require.Contains(t, err.Error(), "0 arguments")
}

func TestConstant_NonComparisonOp(t *testing.T) {
	tests := []Operator{OpAnd, OpOr, OpNot, OpIn, OpContains}
	for _, op := range tests {
		t.Run(op.String(), func(t *testing.T) {
			_, err := Constant("status", op, "x")(nil)
			require.ErrorIs(t, err, ErrInvalidExpression)
			require.Contains(t, err.Error(), "not a comparison")
		})
	}
}

func TestConstant_ParserPickup(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(map[string]CustomFunction{
		"isActive":    Constant("status", OpEqual, "active"),
		"hasFailures": Constant("failures", OpGT, int64(0)),
	}))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `isActive() && hasFailures()`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpAnd, andNode.Op)

	left := andNode.Left.(*BinaryOpNode)
	require.Equal(t, OpEqual, left.Op)
	require.Equal(t, "status", left.Left.(*IdentNode).Name)
	require.Equal(t, "active", left.Right.(*LiteralNode).Value)

	right := andNode.Right.(*BinaryOpNode)
	require.Equal(t, OpGT, right.Op)
	require.Equal(t, "failures", right.Left.(*IdentNode).Name)
	require.Equal(t, int64(0), right.Right.(*LiteralNode).Value)
}
