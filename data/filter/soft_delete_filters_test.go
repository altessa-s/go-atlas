// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSoftDeleteFilters_NameSet(t *testing.T) {
	got := SoftDeleteFilters()
	require.Len(t, got, 2)
	require.NotNil(t, got[SoftDeleteFuncNotDeleted])
	require.NotNil(t, got[SoftDeleteFuncOnlyDeleted])
}

func TestSoftDeleteFilters_Expansion(t *testing.T) {
	tests := []struct {
		fn     string
		wantOp Operator
	}{
		{SoftDeleteFuncNotDeleted, OpEqual},
		{SoftDeleteFuncOnlyDeleted, OpNotEqual},
	}

	funcs := SoftDeleteFilters()
	for _, tt := range tests {
		t.Run(tt.fn, func(t *testing.T) {
			node, err := funcs[tt.fn](nil)
			require.NoError(t, err)

			bin, ok := node.(*BinaryOpNode)
			require.True(t, ok, "expected BinaryOpNode, got %T", node)
			require.Equal(t, tt.wantOp, bin.Op)

			ident, ok := bin.Left.(*IdentNode)
			require.True(t, ok)
			require.Equal(t, TimestampFieldDeletedAt, ident.Name)

			lit, ok := bin.Right.(*LiteralNode)
			require.True(t, ok)
			require.Nil(t, lit.Value, "right side must be the null literal")
		})
	}
}

func TestSoftDeleteFilters_RejectArgs(t *testing.T) {
	funcs := SoftDeleteFilters()
	for _, name := range []string{SoftDeleteFuncNotDeleted, SoftDeleteFuncOnlyDeleted} {
		t.Run(name, func(t *testing.T) {
			_, err := funcs[name]([]Node{&LiteralNode{Value: "x"}})
			require.ErrorIs(t, err, ErrInvalidExpression)
		})
	}
}

func TestSoftDeleteFilters_FreshMapPerCall(t *testing.T) {
	first := SoftDeleteFilters()
	delete(first, SoftDeleteFuncNotDeleted)

	second := SoftDeleteFilters()
	require.NotNil(t, second[SoftDeleteFuncNotDeleted],
		"each call must return a fresh map")
}

func TestSoftDeleteFilters_ParserPickup(t *testing.T) {
	withCleanGlobalRegistry(t)
	require.NoError(t, RegisterFunctions(SoftDeleteFilters()))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `notDeleted() && status == "active"`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpAnd, andNode.Op)

	left, ok := andNode.Left.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpEqual, left.Op)
	require.Equal(t, TimestampFieldDeletedAt, left.Left.(*IdentNode).Name)
	require.Nil(t, left.Right.(*LiteralNode).Value)
}
