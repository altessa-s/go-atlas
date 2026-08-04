// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// mustTranslator builds a translator and fails the test on any
// construction error. Keeps the success-path tests free of
// error-wiring noise; tests that exercise construction failures call
// [NewTranslator] directly.
func mustTranslator(tb testing.TB, opts ...filter.TranslatorOption) *Translator {
	tb.Helper()
	tr, err := NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslator_BasicComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
		args []any
	}{
		{"equal string", `name == "John"`, "`name` = ?", []any{"John"}},
		{"equal int", `age == 25`, "`age` = ?", []any{int64(25)}},
		{"equal bool", `active == true`, "`active` = ?", []any{true}},
		{"equal float", `price == 10.5`, "`price` = ?", []any{10.5}},
		{"not equal", `status != "deleted"`, "`status` != ?", []any{"deleted"}},
		{"greater than", `age > 18`, "`age` > ?", []any{int64(18)}},
		{"greater or equal", `age >= 18`, "`age` >= ?", []any{int64(18)}},
		{"less than", `age < 65`, "`age` < ?", []any{int64(65)}},
		{"less or equal", `age <= 65`, "`age` <= ?", []any{int64(65)}},
		{"reversed operands", `18 < age`, "? < `age`", []any{int64(18)}},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.args, args)
		})
	}
}

func TestTranslator_NullComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"equal null", `deletedAt == null`, "`deletedAt` IS NULL"},
		{"not equal null", `deletedAt != null`, "`deletedAt` IS NOT NULL"},
		{"null on the left", `null == deletedAt`, "`deletedAt` IS NULL"},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Empty(t, args)
		})
	}
}

func TestTranslator_NullOrderingRejected(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `deletedAt > null`)

	_, _, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
}

func TestTranslator_LogicalOperators(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
		args []any
	}{
		{
			"and",
			`name == "John" && age >= 18`,
			"(`name` = ?) AND (`age` >= ?)",
			[]any{"John", int64(18)},
		},
		{
			"or",
			`status == "active" || status == "pending"`,
			"(`status` = ?) OR (`status` = ?)",
			[]any{"active", "pending"},
		},
		{
			"not on bare ident",
			`!active`,
			"NOT (`active` = true)",
			nil,
		},
		{
			"not on expression",
			`!(age >= 18)`,
			"NOT (`age` >= ?)",
			[]any{int64(18)},
		},
		{
			"chained and preserves argument order",
			`a == 1 && b == 2 && c == 3`,
			"((`a` = ?) AND (`b` = ?)) AND (`c` = ?)",
			[]any{int64(1), int64(2), int64(3)},
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.args, args)
		})
	}
}

func TestTranslator_BareIdentIsBooleanTest(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `active && verified`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "(`active` = true) AND (`verified` = true)", got)
	require.Empty(t, args)
}

func TestTranslator_In(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
		args []any
	}{
		{
			"strings",
			`status in ["active", "pending"]`,
			"`status` IN (?, ?)",
			[]any{"active", "pending"},
		},
		{
			"ints",
			`type in [1, 2, 3]`,
			"`type` IN (?, ?, ?)",
			[]any{int64(1), int64(2), int64(3)},
		},
		{
			"single element",
			`status in ["active"]`,
			"`status` IN (?)",
			[]any{"active"},
		},
		{
			"empty list is match-none",
			`status in []`,
			sqlbase.MatchNone,
			nil,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.args, args)
		})
	}
}

// TestTranslator_EmptyInStillChecksAllowlist guards the rollback path in
// translateIn: the left operand is rendered — and therefore validated —
// before the empty list short-circuits to a constant.
func TestTranslator_EmptyInStillChecksAllowlist(t *testing.T) {
	trans := mustTranslator(t, filter.WithAllowedFields("name"))
	node := testhelpers.MustParseFilter(t, `status in []`)

	_, _, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
}

func TestTranslator_StringFunctions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
		args []any
	}{
		{
			"contains",
			`name.contains("oh")`,
			"position(`name`, ?) > 0",
			[]any{"oh"},
		},
		{
			"contains keeps LIKE wildcards literal",
			`name.contains("100%_x")`,
			"position(`name`, ?) > 0",
			[]any{"100%_x"},
		},
		{
			"startsWith",
			`name.startsWith("Jo")`,
			"startsWith(`name`, ?)",
			[]any{"Jo"},
		},
		{
			"endsWith",
			`name.endsWith("hn")`,
			"endsWith(`name`, ?)",
			[]any{"hn"},
		},
		{
			"matches",
			`name.matches("^Jo.*n$")`,
			"match(`name`, ?)",
			[]any{"^Jo.*n$"},
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.args, args)
		})
	}
}

func TestTranslator_MatchesRejectsOversizedPattern(t *testing.T) {
	trans := mustTranslator(t, filter.WithMaxRegexLength(8))
	node := testhelpers.MustParseFilter(t, `name.matches("aaaaaaaaaaaaaaaaaaaa")`)

	_, _, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrInvalidRegex)
}

func TestTranslator_Has(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `has(user.email)`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "`user.email` IS NOT NULL", got)
	require.Empty(t, args)
}

func TestTranslator_Size(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
		args []any
	}{
		{
			"method form",
			`tags.size() > 2`,
			"length(`tags`) > ?",
			[]any{int64(2)},
		},
		{
			"function form",
			`size(tags) == 0`,
			"length(`tags`) = ?",
			[]any{int64(0)},
		},
		{
			"on the right-hand side",
			`3 <= tags.size()`,
			"? <= length(`tags`)",
			[]any{int64(3)},
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, args, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.args, args)
		})
	}
}

func TestTranslator_UnsupportedOperations(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"substring", `name.substring(0, 3) == "Joh"`},
		{"bare size", `tags.size()`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			_, _, err := trans.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
		})
	}
}

func TestTranslator_NestedFields(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"two levels", `address.city == "NYC"`, "`address.city` = ?"},
		{"three levels", `user.profile.name == "John"`, "`user.profile.name` = ?"},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, _, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_FieldMapping(t *testing.T) {
	trans := mustTranslator(t, filter.WithFieldMapping(map[string]string{
		"organizationIds": "organization_id",
		"createdAt":       "created_at",
	}))
	node := testhelpers.MustParseFilter(t, `organizationIds in ["org-1"] && createdAt > 100`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "(`organization_id` IN (?)) AND (`created_at` > ?)", got)
	require.Equal(t, []any{"org-1", int64(100)}, args)
}

// TestTranslator_FieldMappingCannotEscapeColumnPosition pins the reason
// mapped names are validated rather than merely quoted: the column
// position is the one part of the clause a placeholder cannot cover.
func TestTranslator_FieldMappingCannotEscapeColumnPosition(t *testing.T) {
	tests := []struct {
		name   string
		mapped string
	}{
		{"backtick", "name` = '' OR 1 = 1 -- "},
		{"expression", "lower(name)"},
		{"map subscript", "metadata['x']"},
		{"empty", ""},
		{"empty segment", "address..city"},
		{"leading digit", "1name"},
		{"whitespace", "name OR 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := mustTranslator(t, filter.WithFieldMapping(map[string]string{"name": tt.mapped}))
			node := testhelpers.MustParseFilter(t, `name == "x"`)

			_, _, err := trans.Translate(node)
			require.ErrorIs(t, err, filter.ErrInvalidExpression)
		})
	}
}

func TestTranslator_AllowedFields(t *testing.T) {
	trans := mustTranslator(t, filter.WithAllowedFields("name", "age"))

	t.Run("allowed", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "John" && age > 18`)
		_, _, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("denied", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `passwordHash > ""`)
		_, _, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}

func TestNewTranslator_UntrustedInputRequiresAllowlist(t *testing.T) {
	t.Run("without allowlist", func(t *testing.T) {
		_, err := NewTranslator(filter.WithUntrustedInput())
		require.ErrorIs(t, err, filter.ErrAllowlistRequired)
	})

	t.Run("with allowlist", func(t *testing.T) {
		_, err := NewTranslator(filter.WithUntrustedInput(), filter.WithAllowedFields("name"))
		require.NoError(t, err)
	})
}

func TestTranslator_MaxDepth(t *testing.T) {
	trans := mustTranslator(t, filter.WithMaxDepth(2))
	node := testhelpers.MustParseFilter(t, `a == 1 && b == 2 && c == 3 && d == 4`)

	_, _, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrMaxDepthExceeded)
}

func TestTranslator_NilNodeMatchesAll(t *testing.T) {
	trans := mustTranslator(t)

	where, args, err := trans.Translate(nil)
	require.NoError(t, err)
	require.Equal(t, sqlbase.MatchAll, where)
	require.Empty(t, args)

	inline, err := trans.TranslateInline(nil)
	require.NoError(t, err)
	require.Equal(t, sqlbase.MatchAll, inline)
}

// TestTranslator_ArgsAreNotRetained guards the state reset in run: a
// second Translate must not inherit the previous call's arguments, and
// the slice handed to the first caller must not be reused underneath it.
func TestTranslator_ArgsAreNotRetained(t *testing.T) {
	trans := mustTranslator(t)

	first := testhelpers.MustParseFilter(t, `a == 1 && b == 2`)
	_, firstArgs, err := trans.Translate(first)
	require.NoError(t, err)
	require.Equal(t, []any{int64(1), int64(2)}, firstArgs)

	second := testhelpers.MustParseFilter(t, `c == 3`)
	_, secondArgs, err := trans.Translate(second)
	require.NoError(t, err)
	require.Equal(t, []any{int64(3)}, secondArgs)
	require.Equal(t, []any{int64(1), int64(2)}, firstArgs, "first result was mutated by the second call")
}

// TestTranslator_ArgsClearedAfterError keeps a failed translation from
// leaking half-collected arguments into the next call.
func TestTranslator_ArgsClearedAfterError(t *testing.T) {
	trans := mustTranslator(t, filter.WithAllowedFields("a"))

	failing := testhelpers.MustParseFilter(t, `a == 1 && forbidden == 2`)
	_, args, err := trans.Translate(failing)
	require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	require.Nil(t, args)

	ok := testhelpers.MustParseFilter(t, `a == 9`)
	_, args, err = trans.Translate(ok)
	require.NoError(t, err)
	require.Equal(t, []any{int64(9)}, args)
}

func TestTranslator_Timestamp(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `createdAt > timestamp("2024-01-02T03:04:05.123Z")`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "`createdAt` > ?", got)
	require.Len(t, args, 1)
	ts, ok := args[0].(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2024, time.January, 2, 3, 4, 5, 123000000, time.UTC), ts.UTC())
}

func TestTranslateInline(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"string", `name == "John"`, "`name` = 'John'"},
		{"int", `age == 25`, "`age` = 25"},
		{"float", `price >= 10.5`, "`price` >= 10.5"},
		{"bool true", `active == true`, "`active` = true"},
		{"bool false", `active == false`, "`active` = false"},
		{"null", `deletedAt == null`, "`deletedAt` IS NULL"},
		{"in list", `status in ["a", "b"]`, "`status` IN ('a', 'b')"},
		{"contains", `name.contains("oh")`, "position(`name`, 'oh') > 0"},
		{"matches", `name.matches("^Jo")`, "match(`name`, '^Jo')"},
		{"size", `tags.size() > 2`, "length(`tags`) > 2"},
		{
			"timestamp",
			`createdAt > timestamp("2024-01-02T03:04:05.123Z")`,
			"`createdAt` > toDateTime64('2024-01-02 03:04:05.123', 3, 'UTC')",
		},
		{
			"logical",
			`name == "John" && age >= 18`,
			"(`name` = 'John') AND (`age` >= 18)",
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.TranslateInline(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslateInline_QuotesHostileLiterals covers the escaping that
// inline mode leans on, since it has no placeholder to hide behind.
func TestTranslateInline_QuotesHostileLiterals(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"single quote", `O'Brien`, `'O\'Brien'`},
		{"backslash", `a\b`, `'a\\b'`},
		{"quote break-out attempt", `' OR 1=1 --`, `'\' OR 1=1 --'`},
		{"escaped quote break-out attempt", `\' OR 1=1 --`, `'\\\' OR 1=1 --'`},
		{"newline", "a\nb", `'a\nb'`},
		{"tab", "a\tb", `'a\tb'`},
		{"carriage return", "a\rb", `'a\rb'`},
		{"nul", "a\x00b", `'a\0b'`},
		{"multi-byte rune", "Ünïcödé", `'Ünïcödé'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, quoteString(tt.value))
		})
	}
}

func TestFormatLiteral(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "NULL"},
		{"uint", uint64(7), "7"},
		{"negative int", int64(-7), "-7"},
		{"nan", math.NaN(), "nan"},
		{"positive infinity", math.Inf(1), "inf"},
		{"negative infinity", math.Inf(-1), "-inf"},
		{"bytes", []byte{0xde, 0xad}, "unhex('dead')"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dialect{}.FormatLiteral(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("unsupported", func(t *testing.T) {
		_, err := dialect{}.FormatLiteral(struct{}{})
		require.ErrorIs(t, err, filter.ErrUnsupportedType)
	})
}
