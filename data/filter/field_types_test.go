// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestEvaluator_FieldTypes_Match(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator(
		filter.WithFieldTypes(map[string]filter.FieldKind{
			"status": filter.FieldKindInt,
			"active": filter.FieldKindBool,
			"name":   filter.FieldKindString,
			"price":  filter.FieldKindFloat,
		}),
	)

	data := map[string]any{
		"status": int64(1),
		"active": true,
		"name":   "Alice",
		"price":  150.0,
	}

	tests := []struct {
		expr string
	}{
		{`status == 1`},
		{`status in [1, 2, 3]`},
		{`active == true`},
		{`name == "Alice"`},
		{`price >= 100`},
		{`price >= 100.0`},
		{`status == null`},
		{`other == "anything"`},
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

func TestEvaluator_FieldTypes_Mismatch(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator(
		filter.WithFieldTypes(map[string]filter.FieldKind{
			"status": filter.FieldKindInt,
			"active": filter.FieldKindBool,
			"name":   filter.FieldKindString,
		}),
	)

	data := map[string]any{
		"status": int64(1),
		"active": true,
		"name":   "Alice",
	}

	tests := []struct {
		expr string
	}{
		{`status == "qwer"`},
		{`status == 1.5`},
		{`active == 1`},
		{`name == 42`},
		{`status in [1, "qwer", 3]`},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			_, err = eval.Evaluate(node, data)
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
		})
	}
}

func TestEvaluator_FieldTypes_NoConfig(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	node, err := p.Parse(t.Context(), `status == "qwer"`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"status": int64(1)})
	require.NotErrorIs(t, err, filter.ErrFieldTypeMismatch)
}
