// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mariadb

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
		{"not equal", `status != "deleted"`, "`status` != ?", []any{"deleted"}},
		{"greater than", `age > 18`, "`age` > ?", []any{int64(18)}},
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
			"NOT (`active` = TRUE)",
			nil,
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
			"LOCATE(?, `name`) > 0",
			[]any{"oh"},
		},
		{
			"contains keeps LIKE wildcards literal",
			`name.contains("100%_x")`,
			"LOCATE(?, `name`) > 0",
			[]any{"100%_x"},
		},
		{
			"startsWith",
			`name.startsWith("Jo")`,
			"LOCATE(?, `name`) = 1",
			[]any{"Jo"},
		},
		{
			"matches",
			`name.matches("^Jo.*n$")`,
			"`name` REGEXP ?",
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
// rendering consumes two bind slots for a single CEL operand — MariaDB
// has no endsWith(), so the needle sizes the suffix and is then compared
// to it.
func TestTranslator_EndsWithBindsNeedleTwice(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `name.endsWith("hn")`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "RIGHT(`name`, CHAR_LENGTH(?)) = ?", got)
	require.Equal(t, []any{"hn", "hn"}, args)
}

// TestTranslator_EndsWithArgumentOrder guards the alignment of the extra
// bind against its neighbours: a predicate that emits two placeholders
// must not shift the arguments of whatever follows it.
func TestTranslator_EndsWithArgumentOrder(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `age > 18 && name.endsWith("hn") && status == "x"`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "((`age` > ?) AND (RIGHT(`name`, CHAR_LENGTH(?)) = ?)) AND (`status` = ?)", got)
	require.Equal(t, []any{int64(18), "hn", "hn", "x"}, args)
}

func TestTranslator_Size(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `tags.size() > 2`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "CHAR_LENGTH(`tags`) > ?", got)
	require.Equal(t, []any{int64(2)}, args)
}

func TestTranslator_Has(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `has(user.email)`)

	got, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, "`user`.`email` IS NOT NULL", got)
	require.Empty(t, args)
}

func TestTranslator_QualifiedIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"two levels", `address.city == "NYC"`, "`address`.`city` = ?"},
		{"three levels", `user.profile.name == "John"`, "`user`.`profile`.`name` = ?"},
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

func TestTranslator_FieldMappingCannotEscapeColumnPosition(t *testing.T) {
	tests := []struct {
		name   string
		mapped string
	}{
		{"backtick", "name` = '' OR 1 = 1 -- "},
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
		{"string", `name == "John"`, "`name` = 'John'"},
		{"int", `age == 25`, "`age` = 25"},
		{"float", `price >= 10.5`, "`price` >= 10.5"},
		{"bool", `active == true`, "`active` = TRUE"},
		{"null", `deletedAt == null`, "`deletedAt` IS NULL"},
		{"in list", `status in ["a", "b"]`, "`status` IN ('a', 'b')"},
		{"contains", `name.contains("oh")`, "LOCATE('oh', `name`) > 0"},
		{"endsWith", `name.endsWith("hn")`, "RIGHT(`name`, CHAR_LENGTH('hn')) = 'hn'"},
		{"matches", `name.matches("^Jo")`, "`name` REGEXP '^Jo'"},
		{
			"timestamp",
			`createdAt > timestamp("2024-01-02T03:04:05.123Z")`,
			"`createdAt` > '2024-01-02 03:04:05.123000'",
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
// no placeholder to hide behind. Doubling the quote is what closes the
// break-out; it holds in every sql_mode.
func TestQuoteString(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"single quote", `O'Brien`, `'O''Brien'`},
		{"backslash", `a\b`, `'a\\b'`},
		{"quote break-out attempt", `' OR 1=1 -- `, `''' OR 1=1 -- '`},
		{"escaped quote break-out attempt", `\' OR 1=1 -- `, `'\\'' OR 1=1 -- '`},
		{"newline", "a\nb", `'a\nb'`},
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
		{"false", false, "FALSE"},
		{"uint", uint64(7), "7"},
		{"negative int", int64(-7), "-7"},
		{"bytes", []byte{0xde, 0xad}, "X'dead'"},
		{
			"time is rendered in UTC",
			time.Date(2024, time.January, 2, 6, 4, 5, 0, time.FixedZone("MSK", 3*60*60)),
			"'2024-01-02 03:04:05.000000'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dialect{}.FormatLiteral(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	// MariaDB's DOUBLE cannot hold the non-finite values and has no
	// literal for them, so they are rejected rather than approximated.
	for _, tt := range []struct {
		name  string
		value float64
	}{
		{"nan", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialect{}.FormatLiteral(tt.value)
			require.ErrorIs(t, err, filter.ErrUnsupportedType)
		})
	}

	t.Run("unsupported type", func(t *testing.T) {
		_, err := dialect{}.FormatLiteral(struct{}{})
		require.ErrorIs(t, err, filter.ErrUnsupportedType)
	})
}
