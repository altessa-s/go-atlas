// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func d128(t *testing.T, s string) bson.Decimal128 {
	t.Helper()
	v, err := bson.ParseDecimal128(s)
	require.NoError(t, err)
	return v
}

// TestTranslator_DecimalField: literals compared with a FieldKindDecimal field are sent as exact Decimal128 values.
func TestTranslator_DecimalField(t *testing.T) {
	t.Parallel()
	// A Translator is not safe for concurrent use: each parallel subtest builds its own.
	opts := []filter.TranslatorOption{filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal})}

	tests := []struct {
		name string
		expr string
		want bson.M
	}{
		{name: "float literal", expr: `price >= 0.1`, want: bson.M{"price": bson.M{"$gte": d128(t, "0.1")}}},
		{name: "int literal", expr: `price < 1000000`, want: bson.M{"price": bson.M{"$lt": d128(t, "1000000")}}},
		{name: "decimal string", expr: `price == "12345678901234567.89"`, want: bson.M{"price": d128(t, "12345678901234567.89")}},
		{name: "trailing zeros folded", expr: `price != "12.30"`, want: bson.M{"price": bson.M{"$ne": d128(t, "12.3")}}},
		{name: "null untouched", expr: `price == null`, want: bson.M{"price": nil}},
		{name: "in list", expr: `price in [0.3, 7, "2.50"]`,
			want: bson.M{"price": bson.M{"$in": []any{d128(t, "0.3"), d128(t, "7"), d128(t, "2.5")}}}},
		{name: "other field untouched", expr: `qty > 0.1`, want: bson.M{"qty": bson.M{"$gt": 0.1}}},
		{name: "source spelling", expr: `price == 1.00000000000000001`, want: bson.M{"price": d128(t, "1.00000000000000001")}},
		{name: "17 digits", expr: `price < 0.30000000000000004`, want: bson.M{"price": bson.M{"$lt": d128(t, "0.30000000000000004")}}},
		{name: "in list spelling", expr: `price in [1.00000000000000001, 2]`,
			want: bson.M{"price": bson.M{"$in": []any{d128(t, "1.00000000000000001"), d128(t, "2")}}}},
		{name: "zero with extreme exponent", expr: `price > "0e-999999999"`, want: bson.M{"price": bson.M{"$gt": d128(t, "0")}}},
		{name: "unquoted zero with extreme exponent", expr: `price > 0e-999999999`, want: bson.M{"price": bson.M{"$gt": d128(t, "0")}}},
		{name: "float underflow kept exact", expr: `price > 1e-400`, want: bson.M{"price": bson.M{"$gt": d128(t, "1e-400")}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := mustTranslator(t, opts...).Translate(testhelpers.MustParseFilter(t, tc.expr))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestTranslator_DecimalFieldRejects: literals that are not exact decimals fail the translation.
func TestTranslator_DecimalFieldRejects(t *testing.T) {
	t.Parallel()
	// A Translator is not safe for concurrent use: each parallel subtest builds its own.
	opts := []filter.TranslatorOption{filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal})}

	for _, expr := range []string{
		`price > 0.1234567890123456789012345678901234567`, // more than 34 significant digits
		`price > "0e"`,
		`price > 1e-999999999`,
		`price > "1e999999999"`,
		`price == 1e-2147483649`,
		`price in [1, 1e-2147483649]`,
		`price == "1e-2147483649"`,
		`price == "1e2147483648"`,
		`price > "abc"`,
		`price > true`,
		`price in [1, "x"]`,
		`price > "1e7000"`,
	} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			_, err := mustTranslator(t, opts...).Translate(testhelpers.MustParseFilter(t, expr))
			require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
		})
	}
}

// TestTranslator_DecimalZeroWhenAbsent: an absent decimal field stands for 0, judged against the emitted Decimal128.
func TestTranslator_DecimalZeroWhenAbsent(t *testing.T) {
	t.Parallel()
	// A Translator is not safe for concurrent use: each parallel subtest builds its own.
	opts := []filter.TranslatorOption{
		filter.WithFieldTypes(map[string]filter.FieldKind{"price": filter.FieldKindDecimal}),
		filter.WithZeroWhenAbsent(map[string]filter.FieldKind{"price": filter.FieldKindDecimal}),
	}
	exists := func(b bool) bson.M { return bson.M{"price": bson.M{"$exists": b}} }

	tests := []struct {
		name string
		expr string
		want bson.M
	}{
		{name: "equal zero", expr: `price == "0.00"`, want: bson.M{"$or": bson.A{bson.M{"price": d128(t, "0")}, exists(false)}}},
		{name: "greater than zero", expr: `price > 0`, want: bson.M{"$and": bson.A{bson.M{"price": bson.M{"$gt": d128(t, "0")}}, exists(true)}}},
		{name: "below a positive bound", expr: `price < 0.01`, want: bson.M{"$or": bson.A{bson.M{"price": bson.M{"$lt": d128(t, "0.01")}}, exists(false)}}},
		{name: "at most a negative bound", expr: `price <= -0.01`,
			want: bson.M{"$and": bson.A{bson.M{"price": bson.M{"$lte": d128(t, "-0.01")}}, exists(true)}}},
		{name: "in with zero", expr: `price in [0, 5]`,
			want: bson.M{"$or": bson.A{bson.M{"price": bson.M{"$in": []any{d128(t, "0"), d128(t, "5")}}}, exists(false)}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := mustTranslator(t, opts...).Translate(testhelpers.MustParseFilter(t, tc.expr))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
