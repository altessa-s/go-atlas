// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money_test

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/money"
)

var (
	eur = money.MustParseCurrency("EUR")
	jpy = money.MustParseCurrency("JPY")
	kwd = money.MustParseCurrency("KWD")
)

func mustParse(t *testing.T, amount, code string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, code)
	require.NoError(t, err)
	return m
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCurrency(t *testing.T) {
	t.Parallel()

	for code, units := range map[string]int32{"EUR": 2, "usd": 2, " JPY ": 0, "KWD": 3, "CLF": 4, "RSD": 2, "XAD": 2, "XCG": 2} {
		c, err := money.ParseCurrency(code)
		require.NoError(t, err, code)
		require.Equal(t, units, c.MinorUnits(), code)
	}
	_, err := money.ParseCurrency("XXX1")
	require.ErrorIs(t, err, money.ErrUnknownCurrency)
	for _, withdrawn := range []string{"ANG", "BGN", "HRK"} {
		_, err = money.ParseCurrency(withdrawn)
		require.ErrorIs(t, err, money.ErrUnknownCurrency, withdrawn)
	}
	_, err = money.ParseCurrency("")
	require.ErrorIs(t, err, money.ErrUnknownCurrency)
	require.Panics(t, func() { money.MustParseCurrency("NOPE") })

	pts, err := money.NewCurrency("PTS", 0)
	require.NoError(t, err)
	require.Equal(t, "PTS", pts.String())
	for _, bad := range []struct {
		code  string
		units int32
	}{{"", 2}, {"X", -1}, {"X", money.MaxMinorUnits + 1}} {
		_, err := money.NewCurrency(bad.code, bad.units)
		require.ErrorIs(t, err, money.ErrInvalidCurrency)
	}
	require.True(t, money.Currency{}.IsZero())
}

func TestConstructors(t *testing.T) {
	t.Parallel()

	m, err := money.FromMinor(1230, eur)
	require.NoError(t, err)
	require.Equal(t, "12.30 EUR", m.String())
	minor, err := m.MinorUnits()
	require.NoError(t, err)
	require.Equal(t, int64(1230), minor)

	y, err := money.FromMinor(-500, jpy)
	require.NoError(t, err)
	require.Equal(t, "-500 JPY", y.String())

	_, err = money.FromMinor(1, money.Currency{})
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
	_, err = money.New(decimal.Zero, money.Currency{})
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
	_, err = money.Parse("abc", "EUR")
	require.Error(t, err)
	_, err = money.Parse("1", "???")
	require.ErrorIs(t, err, money.ErrUnknownCurrency)

	require.Equal(t, "0.00 EUR", money.Zero(eur).String())
	require.Equal(t, "1.2345 EUR", mustParse(t, "1.2345", "EUR").String(), "an unrounded amount keeps its places")
}

func TestArithmetic(t *testing.T) {
	t.Parallel()

	a, b := mustParse(t, "10.10", "EUR"), mustParse(t, "0.25", "EUR")
	sum, err := a.Add(b)
	require.NoError(t, err)
	require.Equal(t, "10.35 EUR", sum.String())
	diff, err := b.Sub(a)
	require.NoError(t, err)
	require.Equal(t, "-9.85 EUR", diff.String())
	require.True(t, diff.IsNegative())
	require.Equal(t, "9.85 EUR", diff.Abs().String())
	require.Equal(t, "9.85 EUR", diff.Neg().String())

	cmp, err := a.Cmp(b)
	require.NoError(t, err)
	require.Equal(t, 1, cmp)
	require.True(t, mustParse(t, "1.5", "EUR").Equal(mustParse(t, "1.50", "EUR")))
	require.False(t, mustParse(t, "1.5", "EUR").Equal(mustParse(t, "1.5", "USD")))

	_, err = a.Add(mustParse(t, "1", "USD"))
	require.ErrorIs(t, err, money.ErrCurrencyMismatch)
	_, err = a.Sub(money.Money{})
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
	_, err = a.Cmp(mustParse(t, "1", "JPY"))
	require.ErrorIs(t, err, money.ErrCurrencyMismatch)

	vat := a.Mul(dec("0.2"))
	require.Equal(t, "2.020 EUR", vat.String(), "Mul keeps every place")
	require.False(t, mustParse(t, "1.005", "EUR").IsRounded())
}

func TestRound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		amount string
		mode   money.RoundingMode
		want   string
	}{
		{"0.125", money.HalfEven, "0.12"}, {"0.135", money.HalfEven, "0.14"}, {"-0.125", money.HalfEven, "-0.12"},
		{"0.125", money.HalfUp, "0.13"}, {"-0.125", money.HalfUp, "-0.13"},
		{"0.129", money.Down, "0.12"}, {"-0.129", money.Down, "-0.12"},
		{"0.121", money.Up, "0.13"}, {"-0.121", money.Up, "-0.13"},
		{"0.129", money.Floor, "0.12"}, {"-0.121", money.Floor, "-0.13"},
		{"0.121", money.Ceiling, "0.13"}, {"-0.129", money.Ceiling, "-0.12"},
		{"7.5", money.HalfEven, "7.50"},
	}
	for _, tc := range tests {
		t.Run(tc.mode.String()+"/"+tc.amount, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want+" EUR", mustParse(t, tc.amount, "EUR").Round(tc.mode).String())
		})
	}
	require.Equal(t, "2 JPY", mustParse(t, "2.5", "JPY").Round(money.HalfEven).String())
	require.Equal(t, "1.235 KWD", mustParse(t, "1.2345", "KWD").Round(money.HalfUp).String())
	require.Equal(t, "RoundingMode(42)", money.RoundingMode(42).String())
}

func TestMinorUnitsErrors(t *testing.T) {
	t.Parallel()

	_, err := mustParse(t, "1.001", "EUR").MinorUnits()
	require.ErrorIs(t, err, money.ErrNotRounded)
	_, err = mustParse(t, "1e30", "EUR").MinorUnits()
	require.ErrorIs(t, err, money.ErrOverflow)
	_, err = mustParse(t, "1e1000000000", "EUR").MinorUnits()
	require.ErrorIs(t, err, money.ErrOverflow, "rejected before a billion-digit integer is built")
	_, err = mustParse(t, "1e1000000000", "EUR").Allocate(dec("1"))
	require.ErrorIs(t, err, money.ErrOverflow)
	_, err = mustParse(t, "92233720368547758.08", "EUR").MinorUnits()
	require.ErrorIs(t, err, money.ErrOverflow, "one past MaxInt64 minor units")
	zero, err := mustParse(t, "0e-50", "EUR").MinorUnits()
	require.NoError(t, err)
	require.Zero(t, zero)
	_, err = money.Money{}.MinorUnits()
	require.ErrorIs(t, err, money.ErrInvalidCurrency)

	big, err := money.FromMinor(math.MaxInt64, eur)
	require.NoError(t, err)
	minor, err := big.MinorUnits()
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), minor)
}

func strings(ms []money.Money) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.String()
	}
	return out
}

func TestAllocate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		amount string
		code   string
		ratios []string
		want   []string
	}{
		{"thirds", "100.00", "EUR", []string{"1", "1", "1"}, []string{"33.34 EUR", "33.33 EUR", "33.33 EUR"}},
		{"negative", "-100.00", "EUR", []string{"1", "1", "1"}, []string{"-33.34 EUR", "-33.33 EUR", "-33.33 EUR"}},
		{"largest_remainder", "1.00", "EUR", []string{"0.33", "0.33", "0.34"}, []string{"0.33 EUR", "0.33 EUR", "0.34 EUR"}},
		{"remainder_goes_to_largest", "0.07", "EUR", []string{"3", "7"}, []string{"0.02 EUR", "0.05 EUR"}},
		{"tie_goes_to_earlier", "0.05", "EUR", []string{"3", "7"}, []string{"0.02 EUR", "0.03 EUR"}},
		{"zero_ratio", "10.00", "EUR", []string{"0", "1"}, []string{"0.00 EUR", "10.00 EUR"}},
		{"zero_amount", "0", "EUR", []string{"1", "2"}, []string{"0.00 EUR", "0.00 EUR"}},
		{"yen", "1000", "JPY", []string{"1", "1", "1"}, []string{"334 JPY", "333 JPY", "333 JPY"}},
		{"fractional_ratios", "10.00", "EUR", []string{"0.5", "0.25", "0.25"}, []string{"5.00 EUR", "2.50 EUR", "2.50 EUR"}},
		{"one_unit_many_parts", "0.01", "EUR", []string{"1", "1", "1"}, []string{"0.01 EUR", "0.00 EUR", "0.00 EUR"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ratios := make([]decimal.Decimal, len(tc.ratios))
			for i, r := range tc.ratios {
				ratios[i] = dec(r)
			}
			m := mustParse(t, tc.amount, tc.code)
			parts, err := m.Allocate(ratios...)
			require.NoError(t, err)
			require.Equal(t, tc.want, strings(parts))
			requireSum(t, m, parts)
		})
	}
}

func requireSum(t *testing.T, m money.Money, parts []money.Money) {
	t.Helper()
	sum := money.Zero(m.Currency())
	for _, p := range parts {
		var err error
		sum, err = sum.Add(p)
		require.NoError(t, err)
	}
	require.True(t, sum.Equal(m), "parts sum to %s, want %s", sum, m)
}

func TestAllocateErrors(t *testing.T) {
	t.Parallel()

	m := mustParse(t, "10.00", "EUR")
	_, err := m.Allocate()
	require.ErrorIs(t, err, money.ErrInvalidRatios)
	_, err = m.Allocate(dec("0"), dec("0"))
	require.ErrorIs(t, err, money.ErrInvalidRatios)
	_, err = m.Allocate(dec("1"), dec("-1"))
	require.ErrorIs(t, err, money.ErrInvalidRatios)
	_, err = mustParse(t, "10.001", "EUR").Allocate(dec("1"))
	require.ErrorIs(t, err, money.ErrNotRounded)
	_, err = m.Split(0)
	require.ErrorIs(t, err, money.ErrInvalidRatios)
}

func TestSplit(t *testing.T) {
	t.Parallel()

	m := mustParse(t, "10.00", "EUR")
	parts, err := m.Split(3)
	require.NoError(t, err)
	require.Equal(t, []string{"3.34 EUR", "3.33 EUR", "3.33 EUR"}, strings(parts))
	requireSum(t, m, parts)
}

// FuzzAllocate checks that the parts always sum to the total and each part
// stays within one minor unit of its exact share.
func FuzzAllocate(f *testing.F) {
	f.Add(int64(10000), uint16(1), uint16(1), uint16(1))
	f.Add(int64(-7), uint16(3), uint16(0), uint16(5))
	f.Add(int64(1), uint16(65535), uint16(1), uint16(2))
	f.Fuzz(func(t *testing.T, minor int64, r1, r2, r3 uint16) {
		if r1 == 0 && r2 == 0 && r3 == 0 {
			return
		}
		m, err := money.FromMinor(minor, eur)
		require.NoError(t, err)
		ratios := []decimal.Decimal{decimal.NewFromInt(int64(r1)), decimal.NewFromInt(int64(r2)), decimal.NewFromInt(int64(r3))}
		parts, err := m.Allocate(ratios...)
		require.NoError(t, err)
		requireSum(t, m, parts)

		total := decimal.NewFromInt(int64(r1) + int64(r2) + int64(r3))
		for i, p := range parts {
			exact := m.Amount().Mul(ratios[i]).Div(total)
			require.True(t, p.Amount().Sub(exact).Abs().LessThan(dec("0.01")), "part %s too far from %s", p, exact)
		}
	})
}
