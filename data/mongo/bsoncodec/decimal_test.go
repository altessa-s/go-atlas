// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
)

type money struct {
	Amount   decimal.Decimal     `bson:"amount"`
	Fee      *decimal.Decimal    `bson:"fee"`
	Discount decimal.NullDecimal `bson:"discount"`
}

func TestDecimalRoundTrip(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"0", "-0.01", "123.4500", "1e-6176", "9999999999999999999999999999999999",
		"12345678901234567890123456789012340000000", // trailing zeros fold into the exponent
		"-3.14159265358979323846264338327950",
	} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			in := money{Amount: decimal.RequireFromString(s), Discount: decimal.NewNullDecimal(decimal.RequireFromString(s))}
			raw, err := marshal(in)
			require.NoError(t, err)
			require.Equal(t, bson.TypeDecimal128, bson.Raw(raw).Lookup("amount").Type, "stored as Decimal128")
			require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("fee").Type, "nil pointer is null")

			var out money
			require.NoError(t, unmarshal(raw, &out))
			require.True(t, in.Amount.Equal(out.Amount), "%s != %s", in.Amount, out.Amount)
			require.Nil(t, out.Fee)
			require.True(t, out.Discount.Valid)
			require.True(t, in.Amount.Equal(out.Discount.Decimal))
		})
	}
}

func TestDecimalPointerAndNull(t *testing.T) {
	t.Parallel()

	fee := decimal.RequireFromString("1.25")
	raw, err := marshal(money{Fee: &fee})
	require.NoError(t, err)
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("discount").Type, "invalid NullDecimal is null")

	var out money
	require.NoError(t, unmarshal(raw, &out))
	require.NotNil(t, out.Fee)
	require.True(t, fee.Equal(*out.Fee))
	require.False(t, out.Discount.Valid)
}

func TestDecimalOutOfRange(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"1." + strings.Repeat("1", 34), // 35 significant digits
		"1e6145",                       // exponent above the Decimal128 range
		"1e-6177",                      // below it
	} {
		_, err := marshal(money{Amount: decimal.RequireFromString(s)})
		require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange, s)
	}
}

func TestDecimalExtremeExponentFailsFast(t *testing.T) {
	t.Parallel()

	// Formatting this value would allocate billions of zeros.
	_, err := marshal(money{Amount: decimal.New(1, -2147483648)})
	require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange)
	_, err = marshal(money{Amount: decimal.New(1, 2147483647)})
	require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange)
}

func TestDecimalDecodeSources(t *testing.T) {
	t.Parallel()

	decode := func(v any) (money, error) {
		raw, err := bson.Marshal(bson.M{"amount": v})
		require.NoError(t, err)
		var out money
		return out, unmarshal(raw, &out)
	}

	out, err := decode(int32(7))
	require.NoError(t, err)
	require.True(t, decimal.NewFromInt(7).Equal(out.Amount))

	out, err = decode(int64(1) << 40)
	require.NoError(t, err)
	require.True(t, decimal.NewFromInt(1<<40).Equal(out.Amount))

	_, err = decode(1.5)
	require.Error(t, err, "a double is not silently converted")

	nan, err := bson.ParseDecimal128("NaN")
	require.NoError(t, err)
	_, err = decode(nan)
	require.ErrorIs(t, err, bsoncodec.ErrDecimalNotFinite)

	inf, err := bson.ParseDecimal128("-Infinity")
	require.NoError(t, err)
	_, err = decode(inf)
	require.ErrorIs(t, err, bsoncodec.ErrDecimalNotFinite)

	out, err = decode(nil)
	require.NoError(t, err)
	require.True(t, out.Amount.IsZero(), "null decodes to zero")
}
