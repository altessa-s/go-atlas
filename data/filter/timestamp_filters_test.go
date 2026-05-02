// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimestampFilters_NameSet(t *testing.T) {
	got := TimestampFilters()
	wantNames := []string{
		TimestampFuncCreateAfter, TimestampFuncCreateBefore,
		TimestampFuncUpdateAfter, TimestampFuncUpdateBefore,
		TimestampFuncDeleteAfter, TimestampFuncDeleteBefore,
	}
	require.Len(t, got, len(wantNames))
	for _, name := range wantNames {
		require.NotNil(t, got[name], "missing handler for %q", name)
	}
}

func TestTimestampFilters_Expansion(t *testing.T) {
	tests := []struct {
		fn        string
		wantField string
		wantOp    Operator
	}{
		{TimestampFuncCreateAfter, TimestampFieldCreatedAt, OpGT},
		{TimestampFuncCreateBefore, TimestampFieldCreatedAt, OpLT},
		{TimestampFuncUpdateAfter, TimestampFieldUpdatedAt, OpGT},
		{TimestampFuncUpdateBefore, TimestampFieldUpdatedAt, OpLT},
		{TimestampFuncDeleteAfter, TimestampFieldDeletedAt, OpGT},
		{TimestampFuncDeleteBefore, TimestampFieldDeletedAt, OpLT},
	}

	funcs := TimestampFilters()
	arg := &LiteralNode{Value: "2024-01-01"}

	for _, tt := range tests {
		t.Run(tt.fn, func(t *testing.T) {
			node, err := funcs[tt.fn]([]Node{arg})
			require.NoError(t, err)

			bin, ok := node.(*BinaryOpNode)
			require.True(t, ok, "expected BinaryOpNode, got %T", node)
			require.Equal(t, tt.wantOp, bin.Op)

			ident, ok := bin.Left.(*IdentNode)
			require.True(t, ok, "Left is not IdentNode, got %T", bin.Left)
			require.Equal(t, tt.wantField, ident.Name)
			require.Same(t, arg, bin.Right, "argument must pass through unchanged")
		})
	}
}

func TestTimestampFilters_FreshMapPerCall(t *testing.T) {
	first := TimestampFilters()
	delete(first, TimestampFuncCreateAfter)

	second := TimestampFilters()
	require.NotNil(t, second[TimestampFuncCreateAfter],
		"each call must return a fresh map; mutating one must not affect another")
}

func TestTimestampFilters_NotRegisteredByDefault(t *testing.T) {
	withCleanGlobalRegistry(t)

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	for _, expr := range []string{
		`createAfter("2024-01-01")`,
		`createBefore("2024-01-01")`,
		`updateAfter("2024-01-01")`,
		`updateBefore("2024-01-01")`,
		`deleteAfter("2024-01-01")`,
		`deleteBefore("2024-01-01")`,
	} {
		t.Run(expr, func(t *testing.T) {
			_, err := p.Parse(t.Context(), expr)
			require.ErrorIs(t, err, ErrUnsupportedOperation,
				"timestamp filters must be opt-in, not auto-registered")
		})
	}
}

func TestTimestampFilters_GlobalRegistration(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(TimestampFilters()))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	node, err := p.Parse(t.Context(), `createAfter("2024-01-01") && deleteBefore("2024-12-31")`)
	require.NoError(t, err)

	andNode, ok := node.(*BinaryOpNode)
	require.True(t, ok)
	require.Equal(t, OpAnd, andNode.Op)
	require.Equal(t, OpGT, andNode.Left.(*BinaryOpNode).Op)
	require.Equal(t, TimestampFieldCreatedAt, andNode.Left.(*BinaryOpNode).Left.(*IdentNode).Name)
	require.Equal(t, OpLT, andNode.Right.(*BinaryOpNode).Op)
	require.Equal(t, TimestampFieldDeletedAt, andNode.Right.(*BinaryOpNode).Left.(*IdentNode).Name)
}

func TestSelectTimestampFilters_Subset(t *testing.T) {
	got := SelectTimestampFilters(
		TimestampFuncCreateAfter,
		TimestampFuncUpdateBefore,
	)

	require.Len(t, got, 2)
	require.NotNil(t, got[TimestampFuncCreateAfter])
	require.NotNil(t, got[TimestampFuncUpdateBefore])
	require.Nil(t, got[TimestampFuncDeleteAfter])
}

func TestSelectTimestampFilters_UnknownSkipped(t *testing.T) {
	got := SelectTimestampFilters(
		TimestampFuncCreateAfter,
		"unknownTimestamp",
		"createdAt", // a field name, not a function name
	)

	require.Len(t, got, 1)
	require.NotNil(t, got[TimestampFuncCreateAfter])
}

func TestSelectTimestampFilters_Empty(t *testing.T) {
	require.Empty(t, SelectTimestampFilters())
}

func TestSelectTimestampFilters_DuplicateNames(t *testing.T) {
	got := SelectTimestampFilters(
		TimestampFuncCreateAfter,
		TimestampFuncCreateAfter,
	)
	require.Len(t, got, 1)
}

func TestSelectTimestampFilters_ParserPickup(t *testing.T) {
	withCleanGlobalRegistry(t)

	require.NoError(t, RegisterFunctions(SelectTimestampFilters(
		TimestampFuncCreateAfter,
		TimestampFuncCreateBefore,
	)))

	p, err := NewParser(WithParserNoCache())
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `createAfter("2024-01-01")`)
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `deleteAfter("2024-01-01")`)
	require.ErrorIs(t, err, ErrUnsupportedOperation,
		"unselected entries must not be registered")
}
