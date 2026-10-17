// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/clickhouse"
	"github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
	"github.com/altessa-s/go-atlas/data/filter/translators/meili"
	"github.com/altessa-s/go-atlas/data/filter/translators/mongo"
	"github.com/altessa-s/go-atlas/data/filter/translators/postgres"
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	flua "github.com/altessa-s/go-atlas/data/filter/translators/lua"
	lua "github.com/yuin/gopher-lua"
)

func TestRedisNotLiteralBypassesAllowlist(t *testing.T) {
	t.Parallel()
	tr, err := redisearch.NewTranslator(nil, filter.WithUntrustedInput(), filter.WithAllowedFields("public"))
	require.NoError(t, err)
	q, err := tr.Translate(testhelpers.MustParseFilter(t, `!"@secret:{classified}"`))
	t.Logf("query=%s error=%v", q, err)
	require.ErrorIs(t, err, filter.ErrInvalidExpression)
}

func TestUnderscoreFunctionBypassesAllowlist(t *testing.T) {
	t.Parallel()
	p, err := filter.NewParser(filter.WithoutGlobalCustomFunctions(), filter.WithAllowedFunctions("contains"), filter.WithCustomFunctions(map[string]filter.CustomFunction{
		"_hidden": filter.Constant("secret", filter.OpEqual, true),
	}))
	require.NoError(t, err)
	_, err = p.Parse(t.Context(), `_hidden()`)
	require.ErrorIs(t, err, filter.ErrFunctionNotAllowed)
}

func TestTranslatorBudgets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, expr string
		options    []filter.TranslatorOption
		want       error
	}{
		{"operations", `x == 1`, []filter.TranslatorOption{filter.WithMaxOperations(1)}, filter.ErrMaxOperationsExceeded},
		{"list depth", `x in [[[1]]]`, []filter.TranslatorOption{filter.WithMaxDepth(2)}, filter.ErrMaxDepthExceeded},
		{"exact budget", `x == 1`, []filter.TranslatorOption{filter.WithMaxOperations(3), filter.WithMaxDepth(1)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); checkTranslatorBudgets(t, tc.expr, tc.options, tc.want) })
	}
}

func checkTranslatorBudgets(t *testing.T, expr string, opts []filter.TranslatorOption, want error) {
	t.Helper()
	n := testhelpers.MustParseFilter(t, expr)
	pg, err := postgres.NewTranslator(opts...)
	require.NoError(t, err)
	ch, err := clickhouse.NewTranslator(opts...)
	require.NoError(t, err)
	ma, err := mariadb.NewTranslator(opts...)
	require.NoError(t, err)
	mo, err := mongo.NewTranslator(opts...)
	require.NoError(t, err)
	me, err := meili.NewTranslator(opts...)
	require.NoError(t, err)
	lu, err := flua.NewTranslator("d", opts...)
	require.NoError(t, err)
	re, err := redisearch.NewTranslator(nil, opts...)
	require.NoError(t, err)
	cases := []struct {
		name string
		run  func() error
	}{
		{"postgres", func() error { _, _, e := pg.Translate(n); return e }},
		{"clickhouse", func() error { _, _, e := ch.Translate(n); return e }},
		{"mariadb", func() error { _, _, e := ma.Translate(n); return e }},
		{"mongo", func() error { _, e := mo.Translate(n); return e }},
		{"meili", func() error { _, e := me.Translate(n); return e }},
		{"lua", func() error { _, e := lu.Translate(n); return e }},
		{"redisearch", func() error { _, e := re.Translate(n); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if want == nil {
				require.NoError(t, tc.run())
			} else {
				require.ErrorIs(t, tc.run(), want)
			}
		})
	}
}

func TestEvaluatorTypedLiterals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, expr string
		data       map[string]any
	}{
		{"timestamp", `createdAt >= timestamp("2024-01-01T00:00:00Z")`, map[string]any{"createdAt": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{"bytes", `blob == b"abc"`, map[string]any{"blob": []byte("abc")}},
		{"bytes order", `blob < b"abd"`, map[string]any{"blob": []byte("abc")}},
		{"timestamp membership across zones", `createdAt in [timestamp("2024-01-01T00:00:00Z")]`, map[string]any{"createdAt": time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("offset", 3600))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := filter.NewEvaluator()
			require.NoError(t, err)
			got, err := e.Evaluate(testhelpers.MustParseFilter(t, tc.expr), tc.data)
			require.NoError(t, err)
			require.True(t, got)
		})
	}
}

func TestEvaluatorListDepthBudget(t *testing.T) {
	t.Parallel()
	e, err := filter.NewEvaluator(filter.WithMaxDepth(2))
	require.NoError(t, err)
	got, err := e.Evaluate(testhelpers.MustParseFilter(t, `[[[[[1]]]]].size() == 1`), nil)
	t.Logf("value=%v error=%v", got, err)
	require.ErrorIs(t, err, filter.ErrMaxDepthExceeded)
}

func TestMongoFieldComparison(t *testing.T) {
	t.Parallel()
	tr, err := mongo.NewTranslator()
	require.NoError(t, err)
	q, err := tr.Translate(testhelpers.MustParseFilter(t, `a == b`))
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	require.Nil(t, q)
}

func TestSQLStringFieldArgument(t *testing.T) {
	t.Parallel()
	tr, err := postgres.NewTranslator()
	require.NoError(t, err)
	q, args, err := tr.Translate(testhelpers.MustParseFilter(t, `a.contains(b)`))
	require.NoError(t, err)
	t.Logf("query=%s args=%#v", q, args)
	require.Empty(t, args, "field reference must not become a bound string literal")
}

func TestLuaBareBoolean(t *testing.T) {
	t.Parallel()
	tr, err := flua.NewTranslator("d")
	require.NoError(t, err)
	q, err := tr.Translate(testhelpers.MustParseFilter(t, `active`))
	require.NoError(t, err)
	l := lua.NewState()
	t.Cleanup(l.Close)
	require.NoError(t, l.DoString(`d = {active = true}; result = `+q))
	require.Equal(t, lua.LTrue, l.GetGlobal("result"), "query=%s", q)
}

func TestLuaNULFollowedByDigit(t *testing.T) {
	t.Parallel()
	tr, err := flua.NewTranslator("d")
	require.NoError(t, err)
	q, err := tr.Translate(testhelpers.MustParseFilter(t, `name == "\x001"`))
	require.NoError(t, err)
	l := lua.NewState()
	t.Cleanup(l.Close)
	d := l.NewTable()
	d.RawSetString("name", lua.LString("\x001"))
	l.SetGlobal("d", d)
	require.NoError(t, l.DoString("result = "+q))
	require.Equal(t, lua.LTrue, l.GetGlobal("result"), "query=%s", q)
}

func TestEnumOverflowRejected(t *testing.T) {
	t.Parallel()
	e, err := filter.NewEvaluator(filter.WithFieldTypes(map[string]filter.FieldKind{"state": filter.FieldKindInt}), filter.WithEnumValues(map[string][]int64{"state": {0, 1}}))
	require.NoError(t, err)
	_, err = e.Evaluate(testhelpers.MustParseFilter(t, `state == 18446744073709551615u`), map[string]any{"state": uint64(18446744073709551615)})
	require.ErrorIs(t, err, filter.ErrEnumValueNotAllowed)
}

func TestRedisBackslashEscaping(t *testing.T) {
	t.Parallel()
	got := redisearch.EscapeTag(`a\|b`)
	t.Logf("escaped=%s", got)
	require.Equal(t, `a\\\|b`, got)
}

func TestLuaUnicodeSize(t *testing.T) {
	t.Parallel()
	tr, err := flua.NewTranslator("d")
	require.NoError(t, err)
	q, err := tr.Translate(testhelpers.MustParseFilter(t, `name.size() == 1`))
	require.NoError(t, err)
	l := lua.NewState()
	t.Cleanup(l.Close)
	d := l.NewTable()
	d.RawSetString("name", lua.LString("é"))
	l.SetGlobal("d", d)
	require.NoError(t, l.DoString("result = "+q))
	require.Equal(t, lua.LTrue, l.GetGlobal("result"), "query=%s", q)
}

func TestRedisSchemaIsFrozen(t *testing.T) {
	t.Parallel()
	schema := map[string]redisearch.FieldType{"x": redisearch.FieldTypeTag}
	tr, err := redisearch.NewTranslator(schema)
	require.NoError(t, err)
	n := testhelpers.MustParseFilter(t, `x == 1`)
	before, err := tr.Translate(n)
	require.NoError(t, err)
	schema["x"] = redisearch.FieldTypeNumeric
	after, err := tr.Translate(n)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestMongoSizeLiteralCannotReferenceHiddenField(t *testing.T) {
	t.Parallel()
	tr, err := mongo.NewTranslator(filter.WithUntrustedInput(), filter.WithAllowedFields("public"))
	require.NoError(t, err)
	_, err = tr.Translate(testhelpers.MustParseFilter(t, `public.size() > "$secret"`))
	require.ErrorIs(t, err, filter.ErrInvalidExpression)
}

func TestQueryBackendsRejectDynamicValues(t *testing.T) {
	t.Parallel()
	for _, expr := range []string{`a == b`, `a in [b]`, `a.contains(b)`} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			node := testhelpers.MustParseFilter(t, expr)
			mo, err := mongo.NewTranslator()
			require.NoError(t, err)
			_, err = mo.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
			me, err := meili.NewTranslator()
			require.NoError(t, err)
			_, err = me.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
			re, err := redisearch.NewTranslator(nil)
			require.NoError(t, err)
			_, err = re.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
		})
	}
}

func TestLuaFieldOperands(t *testing.T) {
	t.Parallel()
	for _, expr := range []string{`active && other`, `a == b`, `a in ["none", b]`, `tags.size() == 2`} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			tr, err := flua.NewTranslator("d")
			require.NoError(t, err)
			q, err := tr.Translate(testhelpers.MustParseFilter(t, expr))
			require.NoError(t, err)
			l := lua.NewState()
			t.Cleanup(l.Close)
			require.NoError(t, l.DoString(`d = {active = true, other = true, a = "same", b = "same", tags = {1, 2}}; result = `+q))
			require.Equal(t, lua.LTrue, l.GetGlobal("result"), q)
		})
	}
}

func TestSQLDynamicRegexRejected(t *testing.T) {
	t.Parallel()
	node := testhelpers.MustParseFilter(t, `name.matches(pattern)`)
	pg, err := postgres.NewTranslator()
	require.NoError(t, err)
	_, _, err = pg.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	ch, err := clickhouse.NewTranslator()
	require.NoError(t, err)
	_, _, err = ch.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	ma, err := mariadb.NewTranslator()
	require.NoError(t, err)
	_, _, err = ma.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
}

func TestSQLDynamicOperandsHonorFieldPolicy(t *testing.T) {
	t.Parallel()
	for _, expr := range []string{`name in [secret]`, `name.contains(secret)`, `name.endsWith(secret)`} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			tr, err := postgres.NewTranslator(filter.WithAllowedFields("name"))
			require.NoError(t, err)
			_, _, err = tr.Translate(testhelpers.MustParseFilter(t, expr))
			require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
		})
	}
}
