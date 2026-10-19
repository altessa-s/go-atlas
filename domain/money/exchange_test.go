// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/money"
)

var (
	rub = money.MustParseCurrency("RUB")
	kzt = money.MustParseCurrency("KZT")
	usd = money.MustParseCurrency("USD")
)

func TestNormalizeRate(t *testing.T) {
	t.Parallel()

	// 5500 KZT per 100 RUB => 55 KZT per 1 RUB.
	r, err := money.NormalizeRate(rub, kzt, dec("5500"), 100)
	require.NoError(t, err)
	require.True(t, r.Value.Equal(dec("55")), "value = %s, want 55", r.Value)
	require.Equal(t, rub, r.Base)
	require.Equal(t, kzt, r.Quote)

	_, err = money.NormalizeRate(rub, kzt, dec("5500"), 0)
	require.ErrorIs(t, err, money.ErrNonPositiveRate)
	_, err = money.NormalizeRate(rub, kzt, dec("5500"), -1)
	require.ErrorIs(t, err, money.ErrNonPositiveRate)
	_, err = money.NormalizeRate(rub, kzt, decimal.Zero, 1)
	require.ErrorIs(t, err, money.ErrNonPositiveRate)
	_, err = money.NormalizeRate(rub, rub, dec("1"), 1)
	require.ErrorIs(t, err, money.ErrPairMismatch)
	_, err = money.NormalizeRate(money.Currency{}, kzt, dec("1"), 1)
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
}

func TestNormalizeRatePrecision(t *testing.T) {
	t.Parallel()

	// A repeating fraction terminates at RateDivisionPrecision digits, rounded half away from zero.
	r, err := money.NormalizeRate(rub, kzt, dec("2"), 3)
	require.NoError(t, err)
	require.Equal(t, "0.6666666666666666666666666667", r.Value.String())
	require.Equal(t, -money.RateDivisionPrecision, r.Value.Exponent())
}

func TestExchangeRateInverse(t *testing.T) {
	t.Parallel()

	// 100 RUB per 1 EUR => 0.01 EUR per 1 RUB.
	r := money.ExchangeRate{Base: eur, Quote: rub, Value: dec("100")}
	inv := r.Inverse()
	require.Equal(t, r.Quote, inv.Base)
	require.Equal(t, r.Base, inv.Quote)
	require.True(t, inv.Value.Equal(dec("0.01")), "inverse value = %s, want 0.01", inv.Value)

	third := money.ExchangeRate{Base: eur, Quote: rub, Value: dec("3")}.Inverse()
	require.Equal(t, "0.3333333333333333333333333333", third.Value.String())
}

func TestCrossRate(t *testing.T) {
	t.Parallel()

	// EUR/RUB = 100, USD/RUB = 80 (both quoted in RUB) => USD per 1 EUR = 1.25.
	eurRub := money.ExchangeRate{Base: eur, Quote: rub, Value: dec("100")}
	usdRub := money.ExchangeRate{Base: usd, Quote: rub, Value: dec("80")}
	cross, err := money.CrossRate(eurRub, usdRub)
	require.NoError(t, err)
	require.Equal(t, eur, cross.Base)
	require.Equal(t, usd, cross.Quote)
	require.True(t, cross.Value.Equal(dec("1.25")), "cross value = %s, want 1.25", cross.Value)

	// A mismatched common currency is rejected.
	other := money.ExchangeRate{Base: usd, Quote: eur, Value: dec("0.9")}
	_, err = money.CrossRate(eurRub, other)
	require.ErrorIs(t, err, money.ErrPairMismatch)

	// Invalid components are rejected.
	_, err = money.CrossRate(money.ExchangeRate{Base: eur, Quote: rub}, usdRub)
	require.ErrorIs(t, err, money.ErrNonPositiveRate)
	_, err = money.CrossRate(eurRub, money.ExchangeRate{Quote: rub, Value: dec("1")})
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
}

func TestExchangeRateValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rate money.ExchangeRate
		want error
	}{
		{name: "valid", rate: money.ExchangeRate{Base: eur, Quote: rub, Value: dec("100")}},
		{name: "base equals quote", rate: money.ExchangeRate{Base: rub, Quote: rub, Value: dec("1")}, want: money.ErrPairMismatch},
		{name: "zero value", rate: money.ExchangeRate{Base: eur, Quote: rub, Value: decimal.Zero}, want: money.ErrNonPositiveRate},
		{name: "negative value", rate: money.ExchangeRate{Base: eur, Quote: rub, Value: dec("-1")}, want: money.ErrNonPositiveRate},
		{name: "no base", rate: money.ExchangeRate{Quote: rub, Value: dec("1")}, want: money.ErrInvalidCurrency},
		{name: "no quote", rate: money.ExchangeRate{Base: eur, Value: dec("1")}, want: money.ErrInvalidCurrency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.rate.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestExchangeRateString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "100 RUB per 1 EUR", money.ExchangeRate{Base: eur, Quote: rub, Value: dec("100")}.String())
}

func TestConvert(t *testing.T) {
	t.Parallel()

	rate := money.ExchangeRate{Base: usd, Quote: rub, Value: dec("81.2345")}

	got, err := mustParse(t, "10.01", "USD").Convert(rate, money.HalfUp)
	require.NoError(t, err)
	// 10.01 * 81.2345 = 813.157345 -> 813.16 RUB.
	require.Equal(t, "813.16 RUB", got.String())

	down, err := mustParse(t, "10.01", "USD").Convert(rate, money.Down)
	require.NoError(t, err)
	require.Equal(t, "813.15 RUB", down.String())

	// Rounded to the quote currency's minor units: KWD has three.
	toKWD, err := mustParse(t, "1", "USD").Convert(money.ExchangeRate{Base: usd, Quote: kwd, Value: dec("0.30755")}, money.HalfEven)
	require.NoError(t, err)
	require.Equal(t, "0.308 KWD", toKWD.String())

	_, err = mustParse(t, "1", "EUR").Convert(rate, money.HalfUp)
	require.ErrorIs(t, err, money.ErrCurrencyMismatch)
	_, err = mustParse(t, "1", "USD").Convert(money.ExchangeRate{Base: usd, Quote: rub}, money.HalfUp)
	require.ErrorIs(t, err, money.ErrNonPositiveRate)
	_, err = money.Money{}.Convert(rate, money.HalfUp)
	require.ErrorIs(t, err, money.ErrInvalidCurrency)
}
