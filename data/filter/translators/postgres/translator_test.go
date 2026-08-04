// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

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
		{"equal string", `name == "John"`, `"name" = $1`, []any{"John"}},
		{"equal int", `age == 25`, `"age" = $1`, []any{int64(25)}},
		{"equal bool", `active == true`, `"active" = $1`, []any{true}},
		{"not equal", `status != "deleted"`, `"status" != $1`, []any{"deleted"}},
		{"greater than", `age > 18`, `"age" > $1`, []any{int64(18)}},
		{"less or equal", `age <= 65`, `"age" <= $1`, []any{int64(65)}},
		{"reversed operands", `18 < age`, `$1 < "age"`, []any{int64(18)}},
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
		{"equal null", `deletedAt == null`, `"deletedAt" IS NULL`},
		{"not equal null", `deletedAt != null`, `"deletedAt" IS NOT NULL`},
		{"null on the left", `null == deletedAt`, `"deletedAt" IS NULL`},
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

// TestTranslator_PlaceholdersAreNumberedInOrder is the invariant unique
// to PostgreSQL: a positional dialect can repeat "?", but $n must count
// up in exactly the order the arguments are collected.
func TestTranslator_PlaceholdersAreNumberedInOrder(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t,
		`a == 1 && b in [2, 3] && c.contains("x") && d == 4`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t,
		`(("a" = $1) AND ("b" IN ($2, $3))) AND ((strpos("c", $4) > 0) AND ("d" = $5))`,
		got)
	require.Equal(t, []any{int64(1), int64(2), int64(3), "x", int64(4)}, args)
}

// TestTranslator_NumberingRestartsPerCall guards the state reset: a
// second Translate must begin again at $1, not continue the first
// call's sequence.
func TestTranslator_NumberingRestartsPerCall(t *testing.T) {
	trans := mustTranslator(t)

	first := testhelpers.MustParseFilter(t, `a == 1 && b == 2`)
	got, args, err := trans.Translate(first)
	require.NoError(t, err)
	require.Equal(t, `("a" = $1) AND ("b" = $2)`, got)
	require.Equal(t, []any{int64(1), int64(2)}, args)

	second := testhelpers.MustParseFilter(t, `c == 3`)
	got, args, err = trans.Translate(second)
	require.NoError(t, err)
	require.Equal(t, `"c" = $1`, got)
	require.Equal(t, []any{int64(3)}, args)
}

// TestTranslator_EmptyInDoesNotConsumeANumber pins the rollback in the
// empty-IN path: discarded placeholders must not leave a gap in the
// numbering of the clauses around them.
func TestTranslator_EmptyInDoesNotConsumeANumber(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `status in [] || name == "John"`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `(`+sqlbase.MatchNone+`) OR ("name" = $1)`, got)
	require.Equal(t, []any{"John"}, args)
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
			`("name" = $1) AND ("age" >= $2)`,
			[]any{"John", int64(18)},
		},
		{
			"or",
			`status == "active" || status == "pending"`,
			`("status" = $1) OR ("status" = $2)`,
			[]any{"active", "pending"},
		},
		{
			"not on bare ident",
			`!active`,
			`NOT ("active" = TRUE)`,
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
			`strpos("name", $1) > 0`,
			[]any{"oh"},
		},
		{
			"contains keeps LIKE wildcards literal",
			`name.contains("100%_x")`,
			`strpos("name", $1) > 0`,
			[]any{"100%_x"},
		},
		{
			"startsWith",
			`name.startsWith("Jo")`,
			`starts_with("name", $1)`,
			[]any{"Jo"},
		},
		{
			"matches",
			`name.matches("^Jo.*n$")`,
			`"name" ~ $1`,
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

// TestTranslator_EndsWithBindsNeedleTwice pins the one predicate whose
// rendering consumes two bind slots for a single CEL operand — PostgreSQL
// has no endsWith(), so the needle sizes the suffix and is then compared
// to it. The two placeholders must be numbered consecutively.
func TestTranslator_EndsWithBindsNeedleTwice(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `age > 18 && name.endsWith("hn") && status == "x"`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t,
		`(("age" > $1) AND (right("name", length($2)) = $3)) AND ("status" = $4)`,
		got)
	require.Equal(t, []any{int64(18), "hn", "hn", "x"}, args)
}

func TestTranslator_Size(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `tags.size() > 2`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `length("tags") > $1`, got)
	require.Equal(t, []any{int64(2)}, args)
}

func TestTranslator_Has(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `has(user.email)`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `"user"."email" IS NOT NULL`, got)
	require.Empty(t, args)
}

func TestTranslator_QualifiedIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"two levels", `address.city == "NYC"`, `"address"."city" = $1`},
		{"three levels", `user.profile.name == "John"`, `"user"."profile"."name" = $1`},
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

// TestTranslator_QuotingPreservesCase pins the reason identifiers are
// always quoted: PostgreSQL folds an unquoted name to lower case, which
// would silently target a different column.
func TestTranslator_QuotingPreservesCase(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `createdAt > 1`)

	got, _, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `"createdAt" > $1`, got)
}

func TestTranslator_FieldMapping(t *testing.T) {
	trans := mustTranslator(t, filter.WithFieldMapping(map[string]string{
		"organizationIds": "organization_id",
		"createdAt":       "created_at",
	}))
	node := testhelpers.MustParseFilter(t, `organizationIds in ["org-1"] && createdAt > 100`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `("organization_id" IN ($1)) AND ("created_at" > $2)`, got)
	require.Equal(t, []any{"org-1", int64(100)}, args)
}

func TestTranslator_FieldMappingCannotEscapeColumnPosition(t *testing.T) {
	tests := []struct {
		name   string
		mapped string
	}{
		{"double quote", `name" = '' OR 1 = 1 -- `},
		{"expression", "lower(name)"},
		{"empty", ""},
		{"empty segment", "address..city"},
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

func TestNewTranslator_UntrustedInputRequiresAllowlist(t *testing.T) {
	_, err := NewTranslator(filter.WithUntrustedInput())
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	_, err = NewTranslator(filter.WithUntrustedInput(), filter.WithAllowedFields("name"))
	require.NoError(t, err)
}

func TestTranslateInline(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"string", `name == "John"`, `"name" = E'John'`},
		{"int", `age == 25`, `"age" = 25`},
		{"float", `price >= 10.5`, `"price" >= 10.5`},
		{"bool", `active == true`, `"active" = TRUE`},
		{"null", `deletedAt == null`, `"deletedAt" IS NULL`},
		{"in list", `status in ["a", "b"]`, `"status" IN (E'a', E'b')`},
		{"contains", `name.contains("oh")`, `strpos("name", E'oh') > 0`},
		{"endsWith", `name.endsWith("hn")`, `right("name", length(E'hn')) = E'hn'`},
		{"matches", `name.matches("^Jo")`, `"name" ~ E'^Jo'`},
		{
			"timestamp",
			`createdAt > timestamp("2024-01-02T03:04:05.123Z")`,
			`"createdAt" > TIMESTAMP WITH TIME ZONE '2024-01-02 03:04:05.123000+00'`,
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

// TestQuoteString covers the escaping inline mode leans on, since it has
// no placeholder to hide behind. The E prefix is what makes one rule
// correct regardless of standard_conforming_strings.
func TestQuoteString(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"single quote", `O'Brien`, `E'O\'Brien'`},
		{"backslash", `a\b`, `E'a\\b'`},
		{"quote break-out attempt", `' OR 1=1 -- `, `E'\' OR 1=1 -- '`},
		{"escaped quote break-out attempt", `\' OR 1=1 -- `, `E'\\\' OR 1=1 -- '`},
		{"newline", "a\nb", `E'a\nb'`},
		{"nul", "a\x00b", `E'a\000b'`},
		{"multi-byte rune", "Ünïcödé", `E'Ünïcödé'`},
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
		{"false", false, "FALSE"},
		{"uint", uint64(7), "7"},
		{"negative int", int64(-7), "-7"},
		{"nan", math.NaN(), "'NaN'::double precision"},
		{"positive infinity", math.Inf(1), "'Infinity'::double precision"},
		{"negative infinity", math.Inf(-1), "'-Infinity'::double precision"},
		{"bytes", []byte{0xde, 0xad}, "decode('dead', 'hex')"},
		{
			"time is rendered in UTC",
			time.Date(2024, time.January, 2, 6, 4, 5, 0, time.FixedZone("MSK", 3*60*60)),
			"TIMESTAMP WITH TIME ZONE '2024-01-02 03:04:05.000000+00'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dialect{}.FormatLiteral(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("unsupported type", func(t *testing.T) {
		_, err := dialect{}.FormatLiteral(struct{}{})
		require.ErrorIs(t, err, filter.ErrUnsupportedType)
	})
}
