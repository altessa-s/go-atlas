// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// The walker itself is covered end-to-end by the clickhouse, mariadb and
// postgres suites, which exercise it through three real dialects. What
// those cannot reach is the walker's behavior when a dialect fails: every
// production dialect succeeds on the inputs the parser can produce. The
// fake below fails on demand so the propagation paths are pinned.

// errDialect is a sentinel returned by failing.
var errDialect = errors.New("dialect failure")

// failing is a dialect whose selected method always fails. The zero
// value succeeds everywhere, so each test opts into exactly one failure.
type failing struct {
	quoteIdent    bool
	formatLiteral bool
	stringPred    bool
}

func (d failing) QuoteIdent(name string) (string, error) {
	if d.quoteIdent {
		return "", errDialect
	}
	return sqlbase.QuoteQualified(name, '"')
}

func (failing) Placeholder(int) string { return "?" }

func (failing) SizeExpr(col string) string { return "length(" + col + ")" }

func (d failing) FormatLiteral(v any) (string, error) {
	if d.formatLiteral {
		return "", errDialect
	}
	if v == nil {
		return "NULL", nil
	}
	return "<literal>", nil
}

func (d failing) StringPredicate(_ filter.Operator, col, needle string, value sqlbase.ValueFunc) (string, error) {
	if d.stringPred {
		return "", errDialect
	}
	arg, err := value(needle)
	if err != nil {
		return "", err
	}
	return col + " ~ " + arg, nil
}

func mustTranslator(tb testing.TB, d sqlbase.Dialect, opts ...filter.TranslatorOption) *sqlbase.Translator {
	tb.Helper()
	tr, err := sqlbase.New(d, opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate_PropagatesDialectErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect failing
		expr    string
	}{
		{"QuoteIdent", failing{quoteIdent: true}, `name == "x"`},
		{"StringPredicate", failing{stringPred: true}, `name.contains("x")`},
		// FormatLiteral is reached in the parameterized mode only through
		// the boolean test a bare identifier compiles to.
		{"FormatLiteral via bare identifier", failing{formatLiteral: true}, `active && verified`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			trans := mustTranslator(t, tt.dialect)
			node := testhelpers.MustParseFilter(t, tt.expr)

			_, _, err := trans.Translate(node)
			require.ErrorIs(t, err, errDialect)
		})
	}
}

func TestTranslateInline_PropagatesFormatLiteralError(t *testing.T) {
	t.Parallel()

	trans := mustTranslator(t, failing{formatLiteral: true})
	node := testhelpers.MustParseFilter(t, `name == "x"`)

	_, err := trans.TranslateInline(node)
	require.ErrorIs(t, err, errDialect)
}

// TestTranslate_ModeIsNotSticky guards the per-call reset of the inline
// flag: a TranslateInline must not leave the walker rendering literals
// in place for the Translate that follows it.
func TestTranslate_ModeIsNotSticky(t *testing.T) {
	t.Parallel()

	trans := mustTranslator(t, failing{})
	node := testhelpers.MustParseFilter(t, `name == "x"`)

	inline, err := trans.TranslateInline(node)
	require.NoError(t, err)
	require.Equal(t, `"name" = <literal>`, inline)

	where, args, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `"name" = ?`, where)
	require.Equal(t, []any{"x"}, args)
}

// TestTranslateInline_CollectsNoArguments pins that inline mode leaves
// the argument slice untouched — a caller that mixed the two modes would
// otherwise bind values the SQL text never references.
func TestTranslateInline_CollectsNoArguments(t *testing.T) {
	t.Parallel()

	trans := mustTranslator(t, failing{})
	node := testhelpers.MustParseFilter(t, `a == 1 && b == 2`)

	_, err := trans.TranslateInline(node)
	require.NoError(t, err)

	_, args, err := trans.Translate(testhelpers.MustParseFilter(t, `c == 3`))
	require.NoError(t, err)
	require.Equal(t, []any{int64(3)}, args)
}

func TestTranslate_RejectsUnsupportedLiteralType(t *testing.T) {
	t.Parallel()

	trans := mustTranslator(t, failing{})
	// Hand-built: the parser only emits literal types the drivers bind,
	// so the guard is unreachable through Parse. It exists for custom
	// functions, which may put an arbitrary value into a LiteralNode.
	node := &filter.BinaryOpNode{
		Op:    filter.OpEqual,
		Left:  &filter.IdentNode{Name: "name"},
		Right: &filter.LiteralNode{Value: struct{}{}},
	}

	_, _, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedType)
}

func TestNew_UntrustedInputRequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := sqlbase.New(failing{}, filter.WithUntrustedInput())
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}

// TestVisitList is the direct exercise of the one Visitor method the
// walker does not reach through Translate on its own. It is public
// surface — the dialect translators embed the walker and therefore
// satisfy filter.Visitor — so a caller can invoke it, and an
// implementation nothing tests is one nothing guarantees.
func TestVisitList(t *testing.T) {
	t.Parallel()

	trans := mustTranslator(t, failing{})

	t.Run("returns the element values in order", func(t *testing.T) {
		t.Parallel()

		got, err := trans.VisitList(&filter.ListNode{Elements: []filter.Node{
			&filter.LiteralNode{Value: int64(1)},
			&filter.LiteralNode{Value: "two"},
			&filter.LiteralNode{Value: nil},
		}})
		require.NoError(t, err)
		require.Equal(t, []any{int64(1), "two", nil}, got)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		got, err := trans.VisitList(&filter.ListNode{})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates an element failure", func(t *testing.T) {
		t.Parallel()

		_, err := trans.VisitList(&filter.ListNode{Elements: []filter.Node{
			&filter.LiteralNode{Value: struct{}{}},
		}})
		require.ErrorIs(t, err, filter.ErrUnsupportedType)
	})
}
