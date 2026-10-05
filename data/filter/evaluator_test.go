// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func newTestParser(t *testing.T) *filter.Parser {
	t.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(t, err, "NewParser")
	return p
}

// mustEvaluator builds an evaluator and fails the test on any
// construction error. Keeps the success-path tests free of
// error-wiring noise; tests that exercise construction failures call
// [filter.NewEvaluator] directly.
func mustEvaluator(tb testing.TB, opts ...filter.TranslatorOption) *filter.Evaluator {
	tb.Helper()
	e, err := filter.NewEvaluator(opts...)
	require.NoError(tb, err)
	return e
}

func TestEvaluator_Comparison(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{
		"name":   "Alice",
		"age":    int64(30),
		"active": true,
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`name == "Alice"`, true},
		{`name == "Bob"`, false},
		{`name != "Bob"`, true},
		{`age == 30`, true},
		{`age > 25`, true},
		{`age < 25`, false},
		{`age >= 30`, true},
		{`age <= 30`, true},
		{`active == true`, true},
		{`active != false`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate(%q)", tt.expr)
			require.Equal(t, tt.want, got, "Evaluate(%q)", tt.expr)
		})
	}
}

func TestEvaluator_Logical(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{
		"status": "active",
		"count":  int64(5),
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`status == "active" && count > 3`, true},
		{`status == "active" && count > 10`, false},
		{`status == "paused" || count > 3`, true},
		{`status == "paused" || count > 10`, false},
		{`!(status == "paused")`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate")
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluator_StringFunctions(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{"name": "hello-world"}

	tests := []struct {
		expr string
		want bool
	}{
		{`name.contains("world")`, true},
		{`name.contains("xyz")`, false},
		{`name.startsWith("hello")`, true},
		{`name.startsWith("world")`, false},
		{`name.endsWith("world")`, true},
		{`name.endsWith("hello")`, false},
		{`name.matches("^hello.*")`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate")
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluator_Substring(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{
		"recipient": "+381607123",
		"age":       int64(30),
		"greeting":  "Привет", // 6 runes, 12 UTF-8 bytes — used to pin rune-indexing
	}

	tests := []struct {
		name    string
		expr    string
		want    bool
		wantErr bool
	}{
		{name: "two-arg prefix slice", expr: `recipient.substring(0, 4) == "+381"`, want: true},
		{name: "two-arg middle slice", expr: `recipient.substring(4, 7) == "607"`, want: true},
		{name: "one-arg suffix slice", expr: `recipient.substring(7) == "123"`, want: true},
		{name: "empty range", expr: `recipient.substring(3, 3) == ""`, want: true},
		{name: "ascii full-length range", expr: `recipient.substring(0, 10) == "+381607123"`, want: true},
		{name: "no match against literal", expr: `recipient.substring(0, 4) == "+1"`, want: false},
		{name: "composed with logical", expr: `recipient.substring(0, 4) == "+381" && recipient.substring(4, 7) == "607"`, want: true},
		// Multibyte: indices count runes, not UTF-8 bytes. greeting is 6 runes
		// (12 bytes); substring(0, 3) must yield the 3-rune prefix.
		{name: "multibyte rune-indexed prefix", expr: `greeting.substring(0, 3) == "При"`, want: true},
		{name: "multibyte rune-indexed full", expr: `greeting.substring(0, 6) == "Привет"`, want: true},
		// Out-of-range here is 7 runes, not 13 bytes — proves the bound is
		// against rune length and not byte length.
		{name: "multibyte end past rune length rejected", expr: `greeting.substring(0, 7)`, wantErr: true},
		{name: "zero args rejected", expr: `recipient.substring()`, wantErr: true},
		{name: "negative start rejected", expr: `recipient.substring(-1, 4)`, wantErr: true},
		{name: "end past length rejected", expr: `recipient.substring(0, 99)`, wantErr: true},
		{name: "start greater than end rejected", expr: `recipient.substring(5, 2)`, wantErr: true},
		{name: "non-string target rejected", expr: `age.substring(0, 1)`, wantErr: true},
		{name: "non-int arg rejected", expr: `recipient.substring("a")`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse(%q)", tt.expr)
			got, err := eval.Evaluate(node, data)
			if tt.wantErr {
				require.Error(t, err, "Evaluate(%q) should fail", tt.expr)
				return
			}
			require.NoError(t, err, "Evaluate(%q)", tt.expr)
			require.Equal(t, tt.want, got, "Evaluate(%q)", tt.expr)
		})
	}
}

func TestEvaluator_In(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{"status": "active"}

	tests := []struct {
		expr string
		want bool
	}{
		{`status in ["active", "running"]`, true},
		{`status in ["paused", "disabled"]`, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate")
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluator_Has(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{
		"name": "test",
		"nested": map[string]any{
			"field": "value",
		},
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`has(data.name)`, true},
		{`has(data.missing)`, false},
		{`has(data.nested)`, true},
	}

	// Wrap data under a "data" key so has() macro works with field selection.
	data = map[string]any{"data": data}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			require.NoError(t, err, "Parse")
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err, "Evaluate")
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluator_NestedFields(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{
		"address": map[string]any{
			"city": "NYC",
		},
	}

	node, err := p.Parse(t.Context(), `address.city == "NYC"`)
	require.NoError(t, err, "Parse")
	got, err := eval.Evaluate(node, data)
	require.NoError(t, err, "Evaluate")
	require.True(t, got, "expected true")
}

func TestEvaluator_AllowedFields(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithAllowedFields("name"))

	data := map[string]any{"name": "test", "secret": "hidden"}

	node, err := p.Parse(t.Context(), `secret == "hidden"`)
	require.NoError(t, err, "Parse")
	_, err = eval.Evaluate(node, data)
	require.Error(t, err, "expected error for disallowed field")
}

func TestEvaluator_FieldMapping(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t,
		filter.WithFieldMapping(map[string]string{
			"userName": "user_name",
		}))

	data := map[string]any{"user_name": "alice"}

	node, err := p.Parse(t.Context(), `userName == "alice"`)
	require.NoError(t, err, "Parse")
	got, err := eval.Evaluate(node, data)
	require.NoError(t, err, "Evaluate")
	require.True(t, got, "expected true")
}

func TestEvaluator_Size(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{"name": "hello"}

	node, err := p.Parse(t.Context(), `name.size() == 5`)
	require.NoError(t, err, "Parse")
	got, err := eval.Evaluate(node, data)
	require.NoError(t, err, "Evaluate")
	require.True(t, got, "expected true")
}

// TestEvaluator_SizeGlobalForm is a regression for a nil dereference: the
// global spelling `size(field)` panicked the Evaluator while `field.size()`
// worked. Both spellings must now agree.
func TestEvaluator_SizeGlobalForm(t *testing.T) {
	t.Parallel()

	p := newTestParser(t)
	data := map[string]any{"name": "hello", "tags": []any{"a", "b"}}

	tests := []struct {
		expr string
		want bool
	}{
		{`size(name) == 5`, true},
		{`size(name) > 5`, false},
		{`size(tags) == 2`, true},
		{`size(tags) > 0 && name.size() == size(name)`, true},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()

			node, err := p.Parse(t.Context(), tc.expr)
			require.NoError(t, err)
			got, err := mustEvaluator(t).Evaluate(node, data)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestEvaluator_CallWithoutTarget pins that a target-less call — from the
// parser (`contains("x")`) or built by hand — is rejected as malformed
// instead of dereferencing a nil Target.
func TestEvaluator_CallWithoutTarget(t *testing.T) {
	t.Parallel()

	p := newTestParser(t)
	data := map[string]any{"name": "hello"}

	for _, expr := range []string{`contains("x")`, `startsWith("x")`, `endsWith("x")`, `matches("x")`, `substring(1)`} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			node, err := p.Parse(t.Context(), expr)
			require.NoError(t, err)
			_, err = mustEvaluator(t).Evaluate(node, data)
			require.ErrorIs(t, err, filter.ErrInvalidExpression)
		})
	}

	t.Run("hand-built size", func(t *testing.T) {
		t.Parallel()

		node := &filter.BinaryOpNode{
			Op:    filter.OpEqual,
			Left:  &filter.CallNode{Op: filter.OpSize, Args: []filter.Node{&filter.IdentNode{Name: "name"}}},
			Right: &filter.LiteralNode{Value: int64(5)},
		}
		_, err := mustEvaluator(t).Evaluate(node, data)
		require.ErrorIs(t, err, filter.ErrInvalidExpression)
	})
}

func TestEvaluator_NilComparison(t *testing.T) {
	p := newTestParser(t)
	eval := mustEvaluator(t)

	data := map[string]any{"name": "test"}

	node, err := p.Parse(t.Context(), `missing == null`)
	require.NoError(t, err, "Parse")
	got, err := eval.Evaluate(node, data)
	require.NoError(t, err, "Evaluate")
	require.True(t, got, "expected true for missing field == null")
}

func TestEvaluator_RegexLengthLimit(t *testing.T) {
	p := newTestParser(t)
	data := map[string]any{"name": "hello"}

	t.Run("short regex is accepted", func(t *testing.T) {
		eval := mustEvaluator(t)
		node, err := p.Parse(t.Context(), `name.matches("^hello")`)
		require.NoError(t, err, "Parse")
		got, err := eval.Evaluate(node, data)
		require.NoError(t, err, "Evaluate")
		require.True(t, got, "expected true")
	})

	t.Run("regex exceeding default limit is rejected", func(t *testing.T) {
		eval := mustEvaluator(t)
		// Build expression with a regex pattern exceeding 1024 bytes
		longPattern := make([]byte, 1025)
		for i := range longPattern {
			longPattern[i] = 'a'
		}
		node := &filter.CallNode{
			Op:     filter.OpMatches,
			Target: &filter.IdentNode{Name: "name"},
			Args:   []filter.Node{&filter.LiteralNode{Value: string(longPattern)}},
		}
		_, err := eval.Evaluate(node, data)
		require.Error(t, err, "expected error for regex exceeding max length")
		require.ErrorIs(t, err, filter.ErrInvalidRegex)
	})

	t.Run("custom regex length limit", func(t *testing.T) {
		eval := mustEvaluator(t,
			filter.WithMaxRegexLength(10))
		node := &filter.CallNode{
			Op:     filter.OpMatches,
			Target: &filter.IdentNode{Name: "name"},
			Args:   []filter.Node{&filter.LiteralNode{Value: "a]long-pattern"}},
		}
		_, err := eval.Evaluate(node, data)
		require.Error(t, err, "expected error for regex exceeding custom max length")
		require.ErrorIs(t, err, filter.ErrInvalidRegex)
	})
}

func TestEvaluator_MaxOperations(t *testing.T) {
	p := newTestParser(t)
	data := map[string]any{"a": int64(1), "b": int64(2), "c": int64(3)}

	t.Run("normal expression within limit", func(t *testing.T) {
		eval := mustEvaluator(t)
		node, err := p.Parse(t.Context(), `a == 1 && b == 2`)
		require.NoError(t, err, "Parse")
		got, err := eval.Evaluate(node, data)
		require.NoError(t, err, "Evaluate")
		require.True(t, got, "expected true")
	})

	t.Run("expression exceeding low limit is rejected", func(t *testing.T) {
		eval := mustEvaluator(t,
			filter.WithMaxOperations(3))
		// a == 1 && b == 2 visits: BinaryOp(&&), BinaryOp(==), Ident(a), Literal(1), BinaryOp(==), ...
		// With limit=3, it should fail after 3 operations
		node, err := p.Parse(t.Context(), `a == 1 && b == 2`)
		require.NoError(t, err, "Parse")
		_, err = eval.Evaluate(node, data)
		require.Error(t, err, "expected error for exceeding max operations")
		require.ErrorIs(t, err, filter.ErrMaxOperationsExceeded)
	})
}

// TestEvaluator_ConcurrentUse shares one Evaluator across parallel subtests,
// each evaluating with its own data. Evaluate used to store the data and the
// depth/operation counters on the Evaluator itself, so concurrent calls raced
// (caught by -race) and could read each other's data or trip each other's
// limits. MaxOperations is set so a single call fits but two interleaved calls
// sharing one counter would not.
func TestEvaluator_ConcurrentUse(t *testing.T) {
	t.Parallel()

	p := newTestParser(t)
	node, err := p.Parse(t.Context(), `name.size() == 5 && age >= 18 && status in ["active", "pending"]`)
	require.NoError(t, err)

	const opsPerCall = 14 // counted node visits for the expression above
	shared := mustEvaluator(t, filter.WithMaxOperations(opsPerCall))

	matching := map[string]any{"name": "Alice", "age": int64(30), "status": "active"}
	nonMatching := map[string]any{"name": "Bob", "age": int64(10), "status": "closed"}

	got, err := shared.Evaluate(node, matching)
	require.NoError(t, err, "a single call must fit within the operation limit")
	require.True(t, got)

	for i := range 8 {
		t.Run("worker", func(t *testing.T) {
			t.Parallel()

			data, want := matching, true
			if i%2 == 1 {
				data, want = nonMatching, false
			}
			for range 500 {
				got, err := shared.Evaluate(node, data)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		})
	}
}
