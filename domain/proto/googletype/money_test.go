// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype_test

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/money"
	"github.com/altessa-s/go-atlas/domain/proto/googletype"

	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

var (
	usd = money.MustParseCurrency("USD")
	rub = money.MustParseCurrency("RUB")
)

func TestMoneyFromProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		p      *gtypemoney.Money
		amount string
		code   string
	}{
		{name: "units and nanos", p: &gtypemoney.Money{CurrencyCode: "USD", Units: 12, Nanos: 345_000_000}, amount: "12.345", code: "USD"},
		{name: "nanos only", p: &gtypemoney.Money{CurrencyCode: "EUR", Nanos: 1}, amount: "0.000000001", code: "EUR"},
		{name: "negative", p: &gtypemoney.Money{CurrencyCode: "USD", Units: -1, Nanos: -500_000_000}, amount: "-1.5", code: "USD"},
		{name: "negative nanos only", p: &gtypemoney.Money{CurrencyCode: "USD", Nanos: -1}, amount: "-0.000000001", code: "USD"},
		{name: "zero", p: &gtypemoney.Money{CurrencyCode: "USD"}, amount: "0", code: "USD"},
		{name: "code parsed like money.ParseCurrency", p: &gtypemoney.Money{CurrencyCode: "usd", Units: 1}, amount: "1", code: "USD"},
		{
			name: "MaxInt64 bound", p: &gtypemoney.Money{CurrencyCode: "USD", Units: math.MaxInt64, Nanos: 999_999_999},
			amount: "9223372036854775807.999999999", code: "USD",
		},
		{
			name: "MinInt64 bound", p: &gtypemoney.Money{CurrencyCode: "USD", Units: math.MinInt64, Nanos: -999_999_999},
			amount: "-9223372036854775808.999999999", code: "USD",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := googletype.MoneyFromProto(tc.p)
			require.NoError(t, err)
			require.True(t, decimal.RequireFromString(tc.amount).Equal(m.Amount()), "amount = %s", m.Amount())
			require.Equal(t, tc.code, m.Currency().Code())
		})
	}

	t.Run("nil is the zero Money", func(t *testing.T) {
		t.Parallel()
		m, err := googletype.MoneyFromProto(nil)
		require.NoError(t, err)
		require.True(t, m.IsZero())
		require.True(t, m.Currency().IsZero())
	})
}

func TestMoneyFromProtoErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    *gtypemoney.Money
		want error
	}{
		{name: "nanos too large", p: &gtypemoney.Money{CurrencyCode: "USD", Nanos: 1_000_000_000}, want: googletype.ErrInvalidNanos},
		{name: "nanos too small", p: &gtypemoney.Money{CurrencyCode: "USD", Units: -1, Nanos: -1_000_000_000}, want: googletype.ErrInvalidNanos},
		{name: "nanos checked before the code", p: &gtypemoney.Money{Nanos: 1_000_000_000}, want: googletype.ErrInvalidNanos},
		{name: "negative units positive nanos", p: &gtypemoney.Money{CurrencyCode: "USD", Units: -1, Nanos: 1}, want: googletype.ErrSignMismatch},
		{name: "positive units negative nanos", p: &gtypemoney.Money{CurrencyCode: "USD", Units: 1, Nanos: -1}, want: googletype.ErrSignMismatch},
		{name: "signs checked before the code", p: &gtypemoney.Money{CurrencyCode: "XQZ", Units: 1, Nanos: -1}, want: googletype.ErrSignMismatch},
		{name: "missing currency", p: &gtypemoney.Money{Units: 1}, want: googletype.ErrMissingCurrency},
		{name: "unknown currency", p: &gtypemoney.Money{CurrencyCode: "XQZ", Units: 1}, want: googletype.ErrUnknownCurrency},
		{name: "blank currency", p: &gtypemoney.Money{CurrencyCode: " ", Units: 1}, want: googletype.ErrUnknownCurrency},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := googletype.MoneyFromProto(tc.p)
			require.ErrorIs(t, err, tc.want)
			require.True(t, m.IsZero())
			require.True(t, m.Currency().IsZero())
		})
	}
}

func TestMoneyToProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		m     money.Money
		units int64
		nanos int32
		code  string
	}{
		{name: "units and nanos", m: mustMoney(t, "12.345", usd), units: 12, nanos: 345_000_000, code: "USD"},
		{name: "negative", m: mustMoney(t, "-1.5", usd), units: -1, nanos: -500_000_000, code: "USD"},
		{name: "one nano", m: mustMoney(t, "0.000000001", usd), nanos: 1, code: "USD"},
		{name: "trailing zeros past nine digits", m: mustMoney(t, "1.0000000010000", usd), units: 1, nanos: 1, code: "USD"},
		{name: "zero", m: money.Zero(rub), code: "RUB"},
		{name: "MaxInt64 bound", m: mustMoney(t, "9223372036854775807.999999999", usd), units: math.MaxInt64, nanos: 999_999_999, code: "USD"},
		{name: "MinInt64 bound", m: mustMoney(t, "-9223372036854775808", usd), units: math.MinInt64, code: "USD"},
		{
			name: "MinInt64 with negative nanos", m: mustMoney(t, "-9223372036854775808.999999999", usd),
			units: math.MinInt64, nanos: -999_999_999, code: "USD",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := googletype.MoneyToProto(tc.m)
			require.NoError(t, err)
			require.Equal(t, tc.units, p.GetUnits())
			require.Equal(t, tc.nanos, p.GetNanos())
			require.Equal(t, tc.code, p.GetCurrencyCode())
		})
	}

	t.Run("zero Money is nil", func(t *testing.T) {
		t.Parallel()
		p, err := googletype.MoneyToProto(money.Money{})
		require.NoError(t, err)
		require.Nil(t, p)
	})
}

func TestMoneyToProtoErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    money.Money
		want error
	}{
		{name: "sub-nano digit", m: mustMoney(t, "0.0000000019", usd), want: googletype.ErrPrecision},
		{name: "negative sub-nano digit", m: mustMoney(t, "-1.0000000001", usd), want: googletype.ErrPrecision},
		{name: "above MaxInt64", m: mustMoney(t, "9223372036854775808", usd), want: googletype.ErrOverflow},
		{name: "far above MaxInt64", m: mustMoney(t, "9223372036854775813.5", usd), want: googletype.ErrOverflow},
		{name: "below MinInt64", m: mustMoney(t, "-9223372036854775809", usd), want: googletype.ErrOverflow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := googletype.MoneyToProto(tc.m)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, p)
		})
	}
}

func TestMoneyRoundTrip(t *testing.T) {
	t.Parallel()

	for _, p := range []*gtypemoney.Money{
		{CurrencyCode: "USD", Units: 12, Nanos: 345_000_000},
		{CurrencyCode: "JPY", Units: -7},
		{CurrencyCode: "EUR", Nanos: -999_999_999},
		{CurrencyCode: "RUB", Units: math.MaxInt64, Nanos: 999_999_999},
		{CurrencyCode: "RUB", Units: math.MinInt64},
		{CurrencyCode: "RUB", Units: math.MinInt64, Nanos: -1},
		{CurrencyCode: "RUB", Units: math.MinInt64, Nanos: -999_999_999},
	} {
		m, err := googletype.MoneyFromProto(p)
		require.NoError(t, err)
		back, err := googletype.MoneyToProto(m)
		require.NoError(t, err)
		require.Equal(t, p.GetUnits(), back.GetUnits())
		require.Equal(t, p.GetNanos(), back.GetNanos())
		require.Equal(t, p.GetCurrencyCode(), back.GetCurrencyCode())
	}
}

func mustMoney(t testing.TB, amount string, c money.Currency) money.Money {
	t.Helper()
	m, err := money.New(decimal.RequireFromString(amount), c)
	require.NoError(t, err)
	return m
}
