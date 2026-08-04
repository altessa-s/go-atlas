// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
)

// countingValue is a ValueFunc that records every call, so a test can
// assert how many operands a template actually bound.
func countingValue(bound *[]string) sqlbase.ValueFunc {
	return func(v any) (string, error) {
		s, _ := v.(string)
		*bound = append(*bound, s)
		return "?", nil
	}
}

// oneBind is the shape of a dialect with a native suffix predicate.
var oneBind = sqlbase.StringPredicates{
	Contains:   "position(%[1]s, %[2]s) > 0",
	StartsWith: "startsWith(%[1]s, %[2]s)",
	EndsWith:   "endsWith(%[1]s, %[2]s)",
	Matches:    "match(%[1]s, %[2]s)",
}

// twoBinds is the shape of a dialect that compiles endsWith to a suffix
// comparison and therefore repeats the operand.
var twoBinds = sqlbase.StringPredicates{
	Contains:           "LOCATE(%[2]s, %[1]s) > 0",
	StartsWith:         "LOCATE(%[2]s, %[1]s) = 1",
	EndsWith:           "RIGHT(%[1]s, CHAR_LENGTH(%[2]s)) = %[3]s",
	Matches:            "%[1]s REGEXP %[2]s",
	EndsWithBindsTwice: true,
}

func TestRenderStringPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tpl  sqlbase.StringPredicates
		op   filter.Operator
		want string
	}{
		{"contains, operand second", oneBind, filter.OpContains, "position(`c`, ?) > 0"},
		{"contains, operand first", twoBinds, filter.OpContains, "LOCATE(?, `c`) > 0"},
		{"startsWith", oneBind, filter.OpStartsWith, "startsWith(`c`, ?)"},
		{"matches as an operator", twoBinds, filter.OpMatches, "`c` REGEXP ?"},
		{"endsWith, native", oneBind, filter.OpEndsWith, "endsWith(`c`, ?)"},
		{"endsWith, compiled", twoBinds, filter.OpEndsWith, "RIGHT(`c`, CHAR_LENGTH(?)) = ?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var bound []string
			got, err := sqlbase.RenderStringPredicate(tt.op, "`c`", "x", countingValue(&bound), tt.tpl)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestRenderStringPredicate_BindCount is the assertion the whole
// EndsWithBindsTwice flag exists for. A template that renders the
// operand twice must bind it twice: one bind short and every argument
// after it shifts, which under PostgreSQL's numbered placeholders
// corrupts the query silently rather than failing.
func TestRenderStringPredicate_BindCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tpl  sqlbase.StringPredicates
		op   filter.Operator
		want []string
	}{
		{"contains binds once", twoBinds, filter.OpContains, []string{"x"}},
		{"startsWith binds once", twoBinds, filter.OpStartsWith, []string{"x"}},
		{"matches binds once", twoBinds, filter.OpMatches, []string{"x"}},
		{"native endsWith binds once", oneBind, filter.OpEndsWith, []string{"x"}},
		{"compiled endsWith binds twice", twoBinds, filter.OpEndsWith, []string{"x", "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var bound []string
			_, err := sqlbase.RenderStringPredicate(tt.op, "`c`", "x", countingValue(&bound), tt.tpl)
			require.NoError(t, err)
			require.Equal(t, tt.want, bound)
		})
	}
}

func TestRenderStringPredicate_RejectsOtherOperators(t *testing.T) {
	t.Parallel()

	var bound []string
	_, err := sqlbase.RenderStringPredicate(filter.OpSize, "`c`", "x", countingValue(&bound), oneBind)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
}

// TestRenderStringPredicate_PropagatesValueError covers the second bind
// failing after the first succeeded — the path only the two-bind shape
// reaches.
func TestRenderStringPredicate_PropagatesValueError(t *testing.T) {
	t.Parallel()

	calls := 0
	value := func(any) (string, error) {
		calls++
		if calls == 2 {
			return "", errDialect
		}
		return "?", nil
	}

	_, err := sqlbase.RenderStringPredicate(filter.OpEndsWith, "`c`", "x", value, twoBinds)
	require.ErrorIs(t, err, errDialect)
}
