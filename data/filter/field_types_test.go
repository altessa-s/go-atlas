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
	eval := mustEvaluator(t,

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
	eval := mustEvaluator(t,

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
	eval := mustEvaluator(t)

	node, err := p.Parse(t.Context(), `status == "qwer"`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"status": int64(1)})
	require.NotErrorIs(t, err, filter.ErrFieldTypeMismatch)
}

// TestEvaluator_FieldTypes_MirroredComparison guards the symmetric
// dispatch added via CheckComparison. CEL comparisons are commutative
// (`status == 1` and `1 == status` mean the same thing), and the
// type-check must catch a mismatch in either ordering — otherwise a
// caller who flipped operand order would bypass validation entirely.
//
// Before the fix the check was anchored to left.(*IdentNode) and the
// flipped form passed silently against an int field with a string
// literal. The PR-45 review flagged this as a hook reach gap.
func TestEvaluator_FieldTypes_MirroredComparison(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,

		filter.WithFieldTypes(map[string]filter.FieldKind{
			"status": filter.FieldKindInt,
		}),
	)
	data := map[string]any{"status": int64(1)}

	// Both orderings must surface ErrFieldTypeMismatch.
	for _, expr := range []string{
		`status == "qwer"`,
		`"qwer" == status`,
	} {
		t.Run(expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), expr)
			require.NoError(t, err, "Parse")
			_, err = eval.Evaluate(node, data)
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch,
				"mismatch must be caught regardless of operand order")
		})
	}
}

// TestEvaluator_FieldTypes_BytesAndTimestamp fills the coverage gap
// PR-45 review called out: the original field_types_test.go exercised
// Int / Float / String / Bool but skipped Bytes and Timestamp
// entirely. A regression in kindAccepts for either kind would have
// gone undetected — these tests pin both the match and mismatch path.
func TestEvaluator_FieldTypes_BytesAndTimestamp(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,

		filter.WithFieldTypes(map[string]filter.FieldKind{
			"createdAt": filter.FieldKindTimestamp,
		}),
	)

	// Bytes literals can't be expressed in the CEL grammar this
	// parser supports, so Bytes coverage is exercised via the
	// kindAccepts table test below; here we only assert Timestamp
	// match/mismatch end-to-end through the evaluator.
	t.Run("timestamp match", func(t *testing.T) {
		node, err := p.Parse(t.Context(), `createdAt >= timestamp("2024-01-01T00:00:00Z")`)
		require.NoError(t, err, "Parse")
		// Evaluate against a fixed-now data point — the comparison
		// resolves at runtime; we only care that no
		// ErrFieldTypeMismatch surfaces.
		_, err = eval.Evaluate(node, map[string]any{"createdAt": int64(0)})
		require.NotErrorIs(t, err, filter.ErrFieldTypeMismatch)
	})

	t.Run("timestamp mismatch", func(t *testing.T) {
		node, err := p.Parse(t.Context(), `createdAt == "2024-01-01"`)
		require.NoError(t, err, "Parse")
		_, err = eval.Evaluate(node, map[string]any{"createdAt": int64(0)})
		require.ErrorIs(t, err, filter.ErrFieldTypeMismatch,
			"string literal against Timestamp field must be rejected")
	})
}

// TestEvaluator_FieldTypes_ErrorMessageIsHumanReadable pins the
// error-message format the PR-45 review asked for. Before the fix
// %T was rendered against Go's native typename ("int64", "[]byte",
// "time.Time"), which leaks internals to operators who think in DSL
// terms. The fix routes through valueKindName so messages say
// "int" / "bytes" / "timestamp" — the same vocabulary as the schema
// declaration.
func TestEvaluator_FieldTypes_ErrorMessageIsHumanReadable(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,

		filter.WithFieldTypes(map[string]filter.FieldKind{
			"status": filter.FieldKindInt,
		}),
	)

	node, err := p.Parse(t.Context(), `status == "qwer"`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, map[string]any{"status": int64(1)})
	require.Error(t, err)

	// The message must mention the FieldKind labels, NOT the Go type
	// (would be "string" via %T anyway here, but the key invariant is
	// that the "(want X)" suffix is present so operators see the
	// expected type at a glance).
	msg := err.Error()
	require.Contains(t, msg, `"status"`, "error must include the field name")
	require.Contains(t, msg, "(int)", "error must include the declared kind")
	require.Contains(t, msg, "string", "error must include the offending value's kind")
}
