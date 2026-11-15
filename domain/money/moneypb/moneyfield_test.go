// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package moneypb_test

import (
	"errors"
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/money"
	"github.com/altessa-s/go-atlas/domain/money/moneypb"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

// requireFieldError fails t unless err is a FieldError on field with reason
// that also wraps cause (when cause is not nil).
func requireFieldError(t *testing.T, err error, field, reason string, cause error) {
	t.Helper()
	fe, ok := errors.AsType[*coreerrs.FieldError](err)
	require.True(t, ok, "err = %v, want a FieldError", err)
	require.Equal(t, field, fe.Field)
	require.Equal(t, reason, fe.Reason)
	if cause != nil {
		require.ErrorIs(t, err, cause)
	}
}

func TestMoneyFromProtoAt(t *testing.T) {
	t.Parallel()

	m, err := moneypb.MoneyFromProtoAt("price", &gtypemoney.Money{CurrencyCode: "USD", Units: -1, Nanos: -500_000_000})
	require.NoError(t, err)
	require.Equal(t, "-1.5", m.Amount().String())

	tests := []struct {
		name          string
		p             *gtypemoney.Money
		field, reason string
		cause         error
	}{
		{"nanos", &gtypemoney.Money{CurrencyCode: "USD", Nanos: 1_000_000_000}, "price.nanos", moneypb.ReasonInvalidNanos, moneypb.ErrInvalidNanos},
		{"sign", &gtypemoney.Money{CurrencyCode: "USD", Units: 1, Nanos: -1}, "price", moneypb.ReasonSignMismatch, moneypb.ErrSignMismatch},
		{"no code", &gtypemoney.Money{Units: 1}, "price.currencyCode", moneypb.ReasonRequired, moneypb.ErrMissingCurrency},
		{
			"unknown code", &gtypemoney.Money{CurrencyCode: "XXZ", Units: 1}, "price.currencyCode", moneypb.ReasonUnknownCurrency,
			moneypb.ErrUnknownCurrency,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := moneypb.MoneyFromProtoAt("price", tc.p)
			requireFieldError(t, err, tc.field, tc.reason, tc.cause)
		})
	}
}

func TestNonNegativeMoneyFromProtoAt(t *testing.T) {
	t.Parallel()

	m, err := moneypb.NonNegativeMoneyFromProtoAt("price", &gtypemoney.Money{CurrencyCode: "USD", Units: 1, Nanos: 500_000_000})
	require.NoError(t, err)
	require.Equal(t, "1.5", m.Amount().String())
	require.Equal(t, usd, m.Currency())

	zero, err := moneypb.NonNegativeMoneyFromProtoAt("price", nil)
	require.NoError(t, err)
	require.True(t, zero.Currency().IsZero())

	_, err = moneypb.NonNegativeMoneyFromProtoAt("price", &gtypemoney.Money{CurrencyCode: "USD", Units: -1})
	requireFieldError(t, err, "price", moneypb.ReasonNegative, nil)

	// A structural violation is reported before the sign check.
	_, err = moneypb.NonNegativeMoneyFromProtoAt("price", &gtypemoney.Money{Units: -1})
	requireFieldError(t, err, "price.currencyCode", moneypb.ReasonRequired, moneypb.ErrMissingCurrency)
}

func TestMoneyToProtoFitted(t *testing.T) {
	t.Parallel()

	require.Nil(t, moneypb.MoneyToProtoFitted(money.Money{}))

	tests := []struct {
		amount string
		units  int64
		nanos  int32
	}{
		{"12.5", 12, 500_000_000},
		{"0.1234567891", 0, 123_456_789},
		{"-0.1234567891", 0, -123_456_789},
		{"0.0000000001", 0, 0},
		{"1e30", math.MaxInt64, 999_999_999},
		{"-1e30", math.MinInt64, -999_999_999},
		// Boundaries around the classification thresholds.
		{"0.000000001", 0, 1},
		{"-0.000000001", 0, -1},
		{"0.0000000009999", 0, 0},
		{"9223372036854775807.999999999", math.MaxInt64, 999_999_999},
		{"-9223372036854775808.999999999", math.MinInt64, -999_999_999},
		{"9223372036854775808", math.MaxInt64, 999_999_999},
		{"-9223372036854775809", math.MinInt64, -999_999_999},
		{"9999999999999999999.5", math.MaxInt64, 999_999_999},
		{"10000000000000000000", math.MaxInt64, 999_999_999},
		{"1234567890123456789.123456789123", 1234567890123456789, 123_456_789},
	}
	for _, tc := range tests {
		t.Run(tc.amount, func(t *testing.T) {
			t.Parallel()
			p := moneypb.MoneyToProtoFitted(mustMoney(t, tc.amount, rub))
			require.Equal(t, "RUB", p.GetCurrencyCode())
			require.Equal(t, tc.units, p.GetUnits())
			require.Equal(t, tc.nanos, p.GetNanos())
		})
	}

	// One nano with trailing coefficient zeros: the digit count of such a
	// coefficient must be exact (decimal.NumDigits undercounts 10^15).
	coefficients := []struct {
		value int64
		exp   int32
		nanos int32
	}{
		{1_000_000_000_000_000, -24, 1},
		{-1_000_000_000_000_000, -24, -1},
		{1_000_000_000_000_000, -25, 0},
		{10_000_000_000_000_000, -25, 1},
		{1_000, -12, 1},
	}
	for _, tc := range coefficients {
		t.Run(decimal.New(tc.value, tc.exp).String(), func(t *testing.T) {
			t.Parallel()
			m, err := money.New(decimal.New(tc.value, tc.exp), rub)
			require.NoError(t, err)
			p := moneypb.MoneyToProtoFitted(m)
			require.Equal(t, "RUB", p.GetCurrencyCode())
			require.Zero(t, p.GetUnits())
			require.Equal(t, tc.nanos, p.GetNanos())
		})
	}

	// Compact extremes are classified before any rescaling. They are built
	// directly: decimal parsing rejects exponents beyond
	// decimal.MaxDecodeExponent.
	extremes := []struct {
		name  string
		value int64
		exp   int32
		units int64
		nanos int32
	}{
		{"1e1000000000", 1, 1_000_000_000, math.MaxInt64, 999_999_999},
		{"-1e1000000000", -1, 1_000_000_000, math.MinInt64, -999_999_999},
		{"1e-1000000000", 1, -1_000_000_000, 0, 0},
		{"-1e-1000000000", -1, -1_000_000_000, 0, 0},
		{"123456789e-1000000000", 123456789, -1_000_000_000, 0, 0},
	}
	for _, tc := range extremes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := money.New(decimal.New(tc.value, tc.exp), rub)
			require.NoError(t, err)
			p := moneypb.MoneyToProtoFitted(m)
			require.Equal(t, "RUB", p.GetCurrencyCode())
			require.Equal(t, tc.units, p.GetUnits())
			require.Equal(t, tc.nanos, p.GetNanos())
		})
	}
}
