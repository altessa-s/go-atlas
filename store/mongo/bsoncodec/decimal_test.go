// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"math/big"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/decimal128"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

type money struct {
	Amount   decimal.Decimal      `bson:"amount"`
	Fee      *decimal.Decimal     `bson:"fee"`
	Discount decimal.NullDecimal  `bson:"discount"`
	NullFee  *decimal.NullDecimal `bson:"null_fee"`
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
			require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("null_fee").Type, "nil NullDecimal pointer is null")

			var out money
			require.NoError(t, unmarshal(raw, &out))
			require.True(t, in.Amount.Equal(out.Amount), "%s != %s", in.Amount, out.Amount)
			require.Nil(t, out.Fee)
			require.Nil(t, out.NullFee)
			require.True(t, out.Discount.Valid)
			require.True(t, in.Amount.Equal(out.Discount.Decimal))
		})
	}
}

func TestDecimalPointerAndNull(t *testing.T) {
	t.Parallel()

	fee := decimal.RequireFromString("1.25")
	nullFee := decimal.NewNullDecimal(decimal.RequireFromString("2.50"))
	raw, err := marshal(money{Fee: &fee, NullFee: &nullFee})
	require.NoError(t, err)
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("discount").Type, "invalid NullDecimal is null")

	var out money
	require.NoError(t, unmarshal(raw, &out))
	require.NotNil(t, out.Fee)
	require.True(t, fee.Equal(*out.Fee))
	require.NotNil(t, out.NullFee)
	require.True(t, out.NullFee.Valid)
	require.True(t, nullFee.Decimal.Equal(out.NullFee.Decimal))
	require.False(t, out.Discount.Valid)
}

func TestDecimalPointerDecodeSources(t *testing.T) {
	t.Parallel()

	d128, err := bson.ParseDecimal128("12.50")
	require.NoError(t, err)
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "double", value: 1.5, wantErr: true},
		{name: "string", value: "12.50", wantErr: true},
		{name: "Decimal128", value: d128},
		{name: "null", value: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := bson.Marshal(bson.M{"decimal": tc.value, "null_decimal": tc.value})
			require.NoError(t, err)
			var out struct {
				Decimal     *decimal.Decimal     `bson:"decimal"`
				NullDecimal *decimal.NullDecimal `bson:"null_decimal"`
			}
			err = unmarshal(raw, &out)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.value == nil {
				require.Nil(t, out.Decimal)
				require.Nil(t, out.NullDecimal)
				return
			}
			require.NotNil(t, out.Decimal)
			require.True(t, decimal.RequireFromString("12.5").Equal(*out.Decimal))
			require.NotNil(t, out.NullDecimal)
			require.True(t, out.NullDecimal.Valid)
			require.True(t, decimal.RequireFromString("12.5").Equal(out.NullDecimal.Decimal))
		})
	}
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

func TestFitsDecimal128(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    decimal.Decimal
		want bool
	}{
		{name: "zero", d: decimal.Zero, want: true},
		{name: "negative cents", d: decimal.RequireFromString("-0.01"), want: true},
		{name: "34 digits", d: decimal.RequireFromString(strings.Repeat("9", 34)), want: true},
		{name: "35 digits", d: decimal.RequireFromString(strings.Repeat("9", 35)), want: false},
		{name: "35 digits with trailing zero", d: decimal.RequireFromString(strings.Repeat("9", 34) + "0"), want: true},
		{name: "35 significant fraction digits", d: decimal.RequireFromString("1." + strings.Repeat("1", 34)), want: false},
		{name: "min exponent", d: decimal.New(1, -6176), want: true},
		{name: "below min exponent", d: decimal.New(1, -6177), want: false},
		{name: "below min exponent folds", d: decimal.New(10, -6177), want: true},
		{name: "max exponent", d: decimal.New(1, 6111), want: true},
		{name: "above max exponent clamps", d: decimal.New(1, 6144), want: true},
		{name: "above max exponent too many digits", d: decimal.New(1, 6145), want: false},
		{name: "extreme negative exponent", d: decimal.New(1, -2147483648), want: false},
		{name: "extreme positive exponent", d: decimal.New(1, 2147483647), want: false},
		{name: "zero with extreme exponent", d: decimal.New(0, 7000)},
		{name: "zero with extreme negative exponent", d: decimal.New(0, -7000)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := decimal128.Fits(tc.d)
			_, err := marshal(money{Amount: tc.d})
			// The predicate is exactly the encoder's verdict.
			require.Equal(t, err == nil, got)
			if err != nil {
				require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange)
			}
			if !strings.HasPrefix(tc.name, "zero with") {
				require.Equal(t, tc.want, got)
			}
		})
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

// TestCanonicalDecimal128 pins the stored form of a fitting decimal: the same value with a bounded coefficient and exponent, so a
// zero or a clamped exponent no longer carries the caller's extreme exponent.
func TestCanonicalDecimal128(t *testing.T) {
	t.Parallel()

	for name, d := range map[string]decimal.Decimal{
		"zero, max exponent":       decimal.New(0, 2147483647),
		"zero, min exponent":       decimal.New(0, -2147483648),
		"plain":                    decimal.RequireFromString("-123.4500"),
		"clamped exponent":         decimal.New(1, 6144),
		"folded negative exponent": decimal.New(10, -6177),
		"trailing zeros":           decimal.RequireFromString(strings.Repeat("9", 34) + "0000"),
		"max":                      decimal.New(1, 6111),
		"min":                      decimal.New(1, -6176),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := decimal128.Canonical(d)
			require.True(t, ok)
			require.True(t, decimal128.Fits(got))
			require.GreaterOrEqual(t, got.Exponent(), int32(-6176))
			require.LessOrEqual(t, got.Exponent(), int32(6111))
			require.LessOrEqual(t, len(got.Coefficient().String()), 35) // 34 digits and a sign
			if d.Sign() == 0 {
				require.True(t, got.Equal(decimal.Zero))
				return
			}
			require.Zero(t, got.Cmp(d), "the value is kept")
		})
	}
	for _, d := range []decimal.Decimal{decimal.New(1, 6145), decimal.New(1, -6177), decimal.RequireFromString(strings.Repeat("9", 35))} {
		_, ok := decimal128.Canonical(d)
		require.False(t, ok)
	}
}

// TestCanonicalDecimal128MatchesDriver pins CanonicalDecimal128 and FitsDecimal128 to the driver's own Decimal128 round trip: the
// canonical form is exactly the coefficient and exponent the driver stores for d and reads back, trailing zeros included.
func TestCanonicalDecimal128MatchesDriver(t *testing.T) {
	t.Parallel()

	for _, d := range decimal128Corpus() {
		d128, fits := bson.ParseDecimal128FromBigInt(d.Coefficient(), int(d.Exponent()))
		require.Equal(t, fits, decimal128.Fits(d), "%s e%d", d.Coefficient(), d.Exponent())
		got, ok := decimal128.Canonical(d)
		require.Equal(t, fits, ok)
		if !fits {
			continue
		}
		coefficient, exp, err := d128.BigInt()
		require.NoError(t, err)
		want := decimal.NewFromBigInt(coefficient, int32(exp)) //nolint:gosec // Decimal128 exponents fit in int32
		if want.IsZero() {
			want = decimal.Zero
		}
		require.Equal(t, want.Coefficient().String(), got.Coefficient().String(), "%s e%d", d.Coefficient(), d.Exponent())
		require.Equal(t, want.Exponent(), got.Exponent(), "%s e%d", d.Coefficient(), d.Exponent())
	}
}

// decimal128Corpus returns decimals around every Decimal128 boundary: coefficient lengths about 34 digits with and without trailing
// zeros, exponents about both ends of the range, zeros with any exponent, and a deterministic random spread.
func decimal128Corpus() []decimal.Decimal {
	coefficients := []string{"0", "1", "-1", "7", "10", "-1200", "123456789", strings.Repeat("9", 33), strings.Repeat("9", 34),
		"-" + strings.Repeat("9", 34), strings.Repeat("9", 35), strings.Repeat("9", 34) + "0", strings.Repeat("9", 34) + "000",
		"1" + strings.Repeat("0", 40), "-1" + strings.Repeat("0", 34), "12" + strings.Repeat("0", 33), "123" + strings.Repeat("0", 33)}
	exponents := []int32{0, 1, -1, 2, -2, 32, -33, 6111, 6110, 6112, 6144, 6145, 6146, -6176, -6175, -6177, -6178, -6200, -6210,
		6077, 6078, 6100, -2147483648, 2147483647}
	var out []decimal.Decimal
	for _, c := range coefficients {
		// Built from the coefficient and exponent: decimal parsing rejects exponents beyond decimal.MaxDecodeExponent.
		coefficient, ok := new(big.Int).SetString(c, 10)
		if !ok {
			panic("decimal128Corpus: bad coefficient " + c)
		}
		for _, e := range exponents {
			out = append(out, decimal.NewFromBigInt(coefficient, e))
		}
	}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test corpus
	for range 2000 {
		var sb strings.Builder
		if rng.IntN(2) == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(byte('1' + rng.IntN(9)))
		for range rng.IntN(36) {
			sb.WriteByte(byte('0' + rng.IntN(10)))
		}
		sb.WriteString(strings.Repeat("0", rng.IntN(6)))
		e := int32(rng.IntN(2*6300) - 6300) //nolint:gosec // bounded
		out = append(out, decimal.RequireFromString(sb.String()+"e"+strconv.Itoa(int(e))))
	}
	return out
}
