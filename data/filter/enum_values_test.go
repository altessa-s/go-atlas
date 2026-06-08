// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestEvaluator_EnumValues_InRange(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {1, 2, 3, 4, 6, 7},
		}))

	data := map[string]any{"role": int64(7)}

	tests := []struct {
		expr string
	}{
		{`role == 1`},
		{`role == 7`},
		{`1 == role`},
		{`role != 2`},
		{`role >= 3`},
		{`role in [1, 2, 3]`},
		{`role == null`},
		{`other == 99`}, // no set declared for "other"
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			_, err = eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate")
		})
	}
}

func TestEvaluator_EnumValues_OutOfRange(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {1, 2, 3, 4, 6, 7},
		}))

	data := map[string]any{"role": int64(7)}

	tests := []struct {
		expr string
	}{
		{`role == 10`},
		{`10 == role`},
		{`role == 5`},
		{`role != 5`},
		{`role > 99`},
		{`role in [1, 5, 7]`},
		{`role in [10]`},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			_, err = eval.Evaluate(node, data)
			require.ErrorIs(t, err, filter.ErrEnumValueNotAllowed)
		})
	}
}

func TestEvaluator_EnumValues_NoConfig(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	node, err := p.Parse(t.Context(), `role == 10`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"role": int64(1)})
	require.NotErrorIs(t, err, filter.ErrEnumValueNotAllowed)
}

// TestEvaluator_EnumValues_WithoutFieldKind guards the CheckLiteralKind
// refactor: the enum set must be enforced even when the field has no
// declared FieldKind, so the FieldKindUnspecified short-circuit must not
// skip the enum check.
func TestEvaluator_EnumValues_WithoutFieldKind(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {1, 2, 3},
		}))

	node, err := p.Parse(t.Context(), `role == 10`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"role": int64(1)})
	require.ErrorIs(t, err, filter.ErrEnumValueNotAllowed)
}

// TestEvaluator_EnumValues_ErrorMessage pins the message: it names the
// field, the offending value, and the sorted allowed set.
func TestEvaluator_EnumValues_ErrorMessage(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {7, 1, 2}, // unsorted on purpose
		}))

	node, err := p.Parse(t.Context(), `role == 10`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"role": int64(1)})
	require.Error(t, err)

	msg := err.Error()
	require.Contains(t, msg, `"role"`, "error must include the field name")
	require.Contains(t, msg, "10", "error must include the offending value")
	require.Contains(t, msg, "[1 2 7]", "error must include the sorted allowed set")
}

// TestEvaluator_EnumValues_UintLiteral covers the uint64 branch of the
// internal enumInt64 normalizer: CEL emits a uint64 LiteralNode for the
// unsigned-suffix form (`Nu`) and must be enforced against the same
// int64-keyed set as a plain integer literal. Values above MaxInt64
// cannot fit the set and are passed through silently — the overflow
// guard exists to keep the normalizer total without producing a
// spurious ErrEnumValueNotAllowed.
func TestEvaluator_EnumValues_UintLiteral(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {1, 2, 3, 4, 6, 7},
		}))

	data := map[string]any{"role": int64(7)}

	t.Run("uint in range", func(t *testing.T) {
		node, err := p.Parse(t.Context(), `role == 7u`)
		require.NoError(t, err, "Parse")
		_, err = eval.Evaluate(node, data)
		require.NoError(t, err, "Evaluate")
	})

	t.Run("uint out of range", func(t *testing.T) {
		node, err := p.Parse(t.Context(), `role == 5u`)
		require.NoError(t, err, "Parse")
		_, err = eval.Evaluate(node, data)
		require.ErrorIs(t, err, filter.ErrEnumValueNotAllowed)
	})

	t.Run("uint overflow above MaxInt64 is skipped", func(t *testing.T) {
		// 9223372036854775808u > math.MaxInt64 — enumInt64 reports
		// (0, false) so the membership check is bypassed rather than
		// producing a false ErrEnumValueNotAllowed.
		node, err := p.Parse(t.Context(), `role == 9223372036854775808u`)
		require.NoError(t, err, "Parse")
		_, err = eval.Evaluate(node, data)
		require.NotErrorIs(t, err, filter.ErrEnumValueNotAllowed)
	})
}
