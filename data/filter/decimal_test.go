// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestDecimalLiteral(t *testing.T) {
	t.Parallel()

	ok := []struct {
		name string
		in   any
		want string
	}{
		{name: "decimal as is", in: decimal.RequireFromString("12.30"), want: "12.3"},
		{name: "int64", in: int64(-42), want: "-42"},
		{name: "uint64 max", in: uint64(math.MaxUint64), want: "18446744073709551615"},
		{name: "int32", in: int32(7), want: "7"},
		{name: "int", in: 9, want: "9"},
		{name: "float 0.1 reads as written", in: 0.1, want: "0.1"},
		{name: "float 0.3", in: 0.3, want: "0.3"},
		{name: "float 15 digits", in: 123456789012.345, want: "123456789012.345"},
		{name: "float large exponent", in: 1e21, want: "1000000000000000000000"},
		{name: "float negative zero", in: math.Copysign(0, -1), want: "0"},
		{name: "string", in: "1500000.50", want: "1500000.5"},
		{name: "string exponent", in: "-1e-2", want: "-0.01"},
		{name: "string 34 digits", in: strings.Repeat("9", 34), want: strings.Repeat("9", 34)},
		{name: "string smallest exponent", in: "1e-6176", want: "1e-6176"},
		{name: "string largest exponent", in: "1e6144", want: "1e6144"},
		{name: "string zero with any exponent", in: "0e-9999", want: "0"},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := filter.DecimalLiteral(tc.in)
			require.NoError(t, err)
			require.True(t, got.Equal(decimal.RequireFromString(tc.want)), "%s != %s", got, tc.want)
		})
	}

	bad := []struct {
		name string
		in   any
	}{
		{name: "float 0.1+0.2 has 17 digits", in: math.Nextafter(0.3, 1)}, // the float64 sum 0.30000000000000004
		{name: "float 16 digits", in: 0.1234567890123456},
		{name: "NaN", in: math.NaN()},
		{name: "infinity", in: math.Inf(1)},
		{name: "not a number", in: "abc"},
		{name: "35 digits", in: strings.Repeat("9", 35)},
		{name: "exponent too small", in: "15e-6177"},
		{name: "exponent too large", in: "1e6146"},
		{name: "huge exponent", in: "1e999999999"},
		{name: "bool", in: true},
		{name: "bytes", in: []byte("1")},
		{name: "nil", in: nil},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := filter.DecimalLiteral(tc.in)
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
		})
	}
}

// decimalEvaluator is an untrusted-input evaluator with a decimal price, as a matcher of money bounds would build it.
func decimalEvaluator(t *testing.T) *filter.Evaluator {
	t.Helper()
	eval, err := filter.NewEvaluator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields("price", "text"),
		filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal, "text": filter.FieldKindString}),
	)
	require.NoError(t, err)
	return eval
}

func TestEvaluatorDecimalExact(t *testing.T) {
	t.Parallel()
	eval := decimalEvaluator(t)
	sum := decimal.RequireFromString("0.1").Add(decimal.RequireFromString("0.2"))

	tests := []struct {
		name  string
		expr  string
		price decimal.Decimal
		want  bool
	}{
		{name: "0.1+0.2 equals 0.3", expr: `price == 0.3`, price: sum, want: true},
		{name: "0.1+0.2 is at most 0.3", expr: `price <= 0.3`, price: sum, want: true},
		{name: "0.1+0.2 is not above 0.3", expr: `price > 0.3`, price: sum, want: false},
		{name: "bound inclusive", expr: `price >= 1000000.5`, price: decimal.RequireFromString("1000000.50"), want: true},
		{name: "one kopeck below", expr: `price >= 1000000.5`, price: decimal.RequireFromString("1000000.49"), want: false},
		{name: "beyond float precision", expr: `price > 9007199254740992`, price: decimal.RequireFromString("9007199254740993"), want: true},
		{name: "decimal string literal", expr: `price == "12345678901234567.89"`, price: decimal.RequireFromString("12345678901234567.89"),
			want: true},
		{name: "decimal string literal differs", expr: `price == "12345678901234567.89"`, price: decimal.RequireFromString("12345678901234567.88"),
			want: false},
		{name: "range", expr: `price >= 100 && price <= 200.5`, price: decimal.RequireFromString("200.50"), want: true},
		{name: "in", expr: `price in [0.3, 7]`, price: sum, want: true},
		{name: "not in", expr: `price in [0.31, 7]`, price: sum, want: false},
		{name: "not equal", expr: `price != 0.3`, price: sum, want: false},
		{name: "mirrored", expr: `0.3 == price`, price: sum, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := eval.Evaluate(testhelpers.MustParseFilter(t, tc.expr), map[string]any{"price": tc.price, "text": "x"})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestEvaluatorDecimalMatchesFloatBehavior: keyword bots and bounds that floats decide correctly keep their result.
func TestEvaluatorDecimalMatchesFloatBehavior(t *testing.T) {
	t.Parallel()
	eval := decimalEvaluator(t)
	floatEval, err := filter.NewEvaluator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields("price", "text"),
		filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindFloat, "text": filter.FieldKindString}),
	)
	require.NoError(t, err)

	exprs := []string{
		`price >= 100000 && price <= 5000000`,
		`text.contains("x") && price > 1000.25`,
		`text.contains("y") || price < 10`,
		`price == 250000`,
	}
	prices := []string{"0", "9.99", "1000.25", "1000.26", "250000", "100000", "5000000", "5000000.01"}
	for _, expr := range exprs {
		node := testhelpers.MustParseFilter(t, expr)
		for _, p := range prices {
			d := decimal.RequireFromString(p)
			want, err := floatEval.Evaluate(node, map[string]any{"price": d.InexactFloat64(), "text": "x"})
			require.NoError(t, err)
			got, err := eval.Evaluate(node, map[string]any{"price": d, "text": "x"})
			require.NoError(t, err)
			require.Equal(t, want, got, "%s with price %s", expr, p)
		}
	}
}

func TestEvaluatorDecimalErrors(t *testing.T) {
	t.Parallel()
	eval := decimalEvaluator(t)
	data := map[string]any{"price": decimal.RequireFromString("1"), "text": "x"}

	for _, expr := range []string{
		`price > 0.1234567890123456789012345678901234567`, // more than 34 significant digits even from the source spelling
		`price > "abc"`, `price > true`, `price in [1, "x"]`,
	} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			_, err := eval.Evaluate(testhelpers.MustParseFilter(t, expr), data)
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
		})
	}

	t.Run("missing price is not ordered", func(t *testing.T) {
		t.Parallel()
		_, err := eval.Evaluate(testhelpers.MustParseFilter(t, `price > 1`), map[string]any{"text": "x"})
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})
	t.Run("missing price equality", func(t *testing.T) {
		t.Parallel()
		got, err := eval.Evaluate(testhelpers.MustParseFilter(t, `price == 1`), map[string]any{"text": "x"})
		require.NoError(t, err)
		require.False(t, got)
	})
}

// TestEvaluatorUndeclaredDecimal: a decimal data value compares exactly even without a declared kind.
func TestEvaluatorUndeclaredDecimal(t *testing.T) {
	t.Parallel()
	eval, err := filter.NewEvaluator()
	require.NoError(t, err)
	sum := decimal.RequireFromString("0.1").Add(decimal.RequireFromString("0.2"))

	got, err := eval.Evaluate(testhelpers.MustParseFilter(t, `amount == 0.3`), map[string]any{"amount": sum})
	require.NoError(t, err)
	require.True(t, got)

	_, err = eval.Evaluate(testhelpers.MustParseFilter(t, `amount == true`), map[string]any{"amount": sum})
	require.ErrorIs(t, err, filter.ErrUnsupportedType)
}

func TestRejectFieldKinds(t *testing.T) {
	t.Parallel()

	ctx, err := filter.NewTranslatorContext(filter.WithFieldTypes(map[string]filter.FieldKind{"a": filter.FieldKindInt, "p": filter.FieldKindDecimal}))
	require.NoError(t, err)
	require.ErrorIs(t, ctx.RejectFieldKinds(filter.FieldKindDecimal), filter.ErrUnsupportedType)
	require.NoError(t, ctx.RejectFieldKinds(filter.FieldKindBytes))

	ctx, err = filter.NewTranslatorContext(filter.WithZeroWhenAbsent(map[string]filter.FieldKind{"p": filter.FieldKindDecimal}))
	require.NoError(t, err)
	require.ErrorIs(t, ctx.RejectFieldKinds(filter.FieldKindDecimal), filter.ErrUnsupportedType)

	ctx, err = filter.NewTranslatorContext()
	require.NoError(t, err)
	require.NoError(t, ctx.RejectFieldKinds(filter.FieldKindDecimal))
	require.Equal(t, "decimal", filter.FieldKindDecimal.String())
}

// TestEvaluatorDecimalSourceSpelling: a float literal compared with a decimal is read from its source spelling, not from the
// float64 it parsed to.
func TestEvaluatorDecimalSourceSpelling(t *testing.T) {
	t.Parallel()
	eval := decimalEvaluator(t)
	one := decimal.RequireFromString("1")
	spelled := decimal.RequireFromString("1.00000000000000001")

	tests := []struct {
		name  string
		expr  string
		price decimal.Decimal
		want  bool
	}{
		{name: "rounds to 1 as a float", expr: `price == 1.00000000000000001`, price: one, want: false},
		{name: "matches its spelling", expr: `price == 1.00000000000000001`, price: spelled, want: true},
		{name: "ordering", expr: `price < 1.00000000000000001`, price: one, want: true},
		{name: "exponent", expr: `price == 1.00000000000000001e2`, price: decimal.RequireFromString("100.000000000000001"), want: true},
		{name: "negative", expr: `price > -1.00000000000000001`, price: decimal.RequireFromString("-1"), want: true},
		{name: "negative with space", expr: `price < - 1.00000000000000001`, price: decimal.RequireFromString("-1"), want: false},
		{name: "17 digits exactly", expr: `price == 0.30000000000000004`, price: decimal.RequireFromString("0.30000000000000004"), want: true},
		{name: "17 digits is not 0.3", expr: `price == 0.30000000000000004`, price: decimal.RequireFromString("0.3"), want: false},
		{name: "in list", expr: `price in [2, 1.00000000000000001]`, price: one, want: false},
		{name: "in list spelled", expr: `price in [2, 1.00000000000000001]`, price: spelled, want: true},
		{name: "mirrored", expr: `1.00000000000000001 == price`, price: one, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := eval.Evaluate(testhelpers.MustParseFilter(t, tc.expr), map[string]any{"price": tc.price, "text": "x"})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestParserRecordsFloatSpelling: only float literals whose shortest round-trip form differs from the source keep their spelling.
func TestParserRecordsFloatSpelling(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		`p == 1.5`:                              "",
		`p == 0.1`:                              "",
		`p == 1.00000000000000001`:              "1.00000000000000001",
		`p == -1.00000000000000001`:             "-1.00000000000000001",
		`"ü" == p && 2.000000000000000001 == q`: "2.000000000000000001",
		`p == 0.30000000000000004`:              "0.30000000000000004",
		`p == 1.0e0`:                            "",
	}
	for expr, want := range tests {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			var got string
			filter.Walk(testhelpers.MustParseFilter(t, expr), func(n filter.Node) bool {
				if lit, ok := n.(*filter.LiteralNode); ok {
					if _, isFloat := lit.Value.(float64); isFloat {
						got = lit.Text
					}
				}
				return true
			})
			require.Equal(t, want, got)
		})
	}
}

// TestDecimalBoundedWork: extreme exponents — in a literal or in a decimal operand — are rejected or canonicalized before any
// comparison, so a hostile filter cannot make the comparison rescale by 10^999999999.
func TestDecimalBoundedWork(t *testing.T) { //nolint:paralleltest // measures allocations
	eval := decimalEvaluator(t)
	data := map[string]any{"price": decimal.RequireFromString("1500000.50"), "text": "x"}

	check := func(t *testing.T, run func()) {
		t.Helper()
		start := time.Now()
		allocs := testing.AllocsPerRun(5, run)
		require.Less(t, time.Since(start), 2*time.Second, "bounded time")
		require.Less(t, allocs, 500.0, "bounded allocations")
	}

	for _, expr := range []string{`price > "0e-999999999"`, `price > "0e999999999"`, `price == "0.000e-6000"`} {
		t.Run(expr, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, expr)
			got, err := eval.Evaluate(node, data)
			require.NoError(t, err)
			require.Equal(t, expr != `price == "0.000e-6000"`, got)
			check(t, func() { _, _ = eval.Evaluate(node, data) })
		})
	}
	for _, expr := range []string{`price > "1e-999999999"`, `price > "1e999999999"`} {
		t.Run(expr, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, expr)
			_, err := eval.Evaluate(node, data)
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
			check(t, func() { _, _ = eval.Evaluate(node, data) })
		})
	}

	t.Run("decimal operands", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `price > 1`)
		zero := map[string]any{"price": decimal.New(0, -999999999), "text": "x"}
		got, err := eval.Evaluate(node, zero)
		require.NoError(t, err)
		require.False(t, got, "a zero with any exponent is 0")
		check(t, func() { _, _ = eval.Evaluate(node, zero) })

		tiny := map[string]any{"price": decimal.New(1, -999999999), "text": "x"}
		_, err = eval.Evaluate(node, tiny)
		require.ErrorIs(t, err, filter.ErrUnsupportedType, "beyond the Decimal128 range")
		check(t, func() { _, _ = eval.Evaluate(node, tiny) })

		_, err = filter.DecimalLiteral(decimal.New(1, 999999999))
		require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
		normalized, err := filter.DecimalLiteral(decimal.New(1500, -2))
		require.NoError(t, err)
		require.Equal(t, int32(0), normalized.Exponent(), "trailing zeros fold into the exponent")
		require.Equal(t, "15", normalized.Coefficient().String())
	})
}

// TestParserBoundedWorkExtremeExponents: parsing float literals with extreme exponents, quoted or not, does bounded work (no
// arithmetic scaled by the exponent), and a decimal field then accepts zero and rejects values outside the Decimal128 range.
func TestParserBoundedWorkExtremeExponents(t *testing.T) { //nolint:paralleltest // measures allocations
	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(t, err)
	eval := decimalEvaluator(t)
	data := map[string]any{"price": decimal.RequireFromString("1500000.50"), "text": "x"}

	tests := []struct {
		expr     string
		parseErr bool // CEL itself rejects the literal
		wantErr  bool // the decimal field rejects the literal
		want     bool
	}{
		{expr: `price > 0e-999999999`, want: true},
		{expr: `price > 0e999999999`, want: true},
		{expr: `price > 0.0e-6177`, want: true},
		{expr: `price > 1e-999999999`, wantErr: true},
		{expr: `price > 1e-400`, want: true}, // underflows the float64 to 0; read exactly from its spelling
		{expr: `price < 1e-400`, want: false},
		{expr: `price > 1e999999999`, parseErr: true},
		{expr: `price == 1e-2147483649`, wantErr: true}, // exponent below MinInt32: the float is 0, the spelling is not a valid decimal
		{expr: `price > -1e-2147483649`, wantErr: true},
		{expr: `price in [7, 1e-2147483649]`, wantErr: true},
		{expr: `price > 1e2147483648`, parseErr: true},
		{expr: `price == "1e-2147483649"`, wantErr: true},
		{expr: `price == "1e2147483648"`, wantErr: true},
		{expr: `price in ["1e-2147483649"]`, wantErr: true},
		{expr: `price > "0e-999999999"`, want: true},
		{expr: `price > "1e-999999999"`, wantErr: true},
		{expr: `price > "1e999999999"`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			start := time.Now()
			allocs := testing.AllocsPerRun(5, func() { _, _ = parser.Parse(t.Context(), tc.expr) })
			require.Less(t, time.Since(start), 2*time.Second, "bounded parse time")
			require.Less(t, allocs, 2000.0, "bounded parse allocations")

			node, err := parser.Parse(t.Context(), tc.expr)
			if tc.parseErr {
				require.ErrorIs(t, err, filter.ErrParseFailed)
				return
			}
			require.NoError(t, err)
			got, err := eval.Evaluate(node, data)
			if tc.wantErr {
				require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestEvaluatorUnrecoveredSpellingRejected: a float literal compared with a decimal whose spelling cannot be certified never falls
// back to its float64 value, even against a zero price that the float would match.
func TestEvaluatorUnrecoveredSpellingRejected(t *testing.T) {
	t.Parallel()
	eval := decimalEvaluator(t)
	zero := map[string]any{"price": decimal.Zero, "text": "x"}
	for _, expr := range []string{`price == 1e-2147483649`, `price in [1e-2147483649]`} {
		_, err := eval.Evaluate(testhelpers.MustParseFilter(t, expr), zero)
		require.ErrorIs(t, err, filter.ErrFieldTypeMismatch, expr)
	}

	// Undeclared decimal data: the comparison fails instead of matching zero.
	undeclared, err := filter.NewEvaluator()
	require.NoError(t, err)
	_, err = undeclared.Evaluate(testhelpers.MustParseFilter(t, `amount == 1e-2147483649`), map[string]any{"amount": decimal.Zero})
	require.ErrorIs(t, err, filter.ErrUnsupportedType)
}
