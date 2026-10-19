// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	// ErrNonPositiveRate is returned when an exchange rate, or a published quote or nominal it is normalized from, is zero or negative.
	// A missing rate is never replaced by zero or one.
	ErrNonPositiveRate = errors.New("money: exchange rate must be positive")
	// ErrPairMismatch is returned when exchange rates do not form the currency pair a computation expects.
	ErrPairMismatch = errors.New("money: currency pair mismatch")
)

// RateDivisionPrecision is the number of fractional digits kept by the divisions of exchange-rate arithmetic ([NormalizeRate],
// [ExchangeRate.Inverse], [CrossRate]), so repeating fractions terminate (rounded half away from zero) without distorting rates stored
// with a dozen digits.
const RateDivisionPrecision int32 = 28

// ExchangeRate is the price of one unit of Base expressed in Quote: Value units of Quote per 1 Base. It is a plain value; [ExchangeRate.Validate]
// checks it.
type ExchangeRate struct {
	Base  Currency
	Quote Currency
	Value decimal.Decimal
}

// Validate checks that the rate names two distinct, set currencies ([ErrInvalidCurrency], [ErrPairMismatch]) and is strictly positive
// ([ErrNonPositiveRate]).
func (r ExchangeRate) Validate() error {
	if r.Base.IsZero() {
		return fmt.Errorf("%w: base", ErrInvalidCurrency)
	}
	if r.Quote.IsZero() {
		return fmt.Errorf("%w: quote", ErrInvalidCurrency)
	}
	if r.Base == r.Quote {
		return fmt.Errorf("%w: base equals quote %s", ErrPairMismatch, r.Base)
	}
	if !r.Value.IsPositive() {
		return fmt.Errorf("%w: %s per 1 %s = %s", ErrNonPositiveRate, r.Quote, r.Base, r.Value)
	}
	return nil
}

// Inverse returns the rate of Base per one unit of Quote: 1 / Value with [RateDivisionPrecision] fractional digits. The rate must be
// positive (see [ExchangeRate.Validate]); Inverse panics on a zero Value.
func (r ExchangeRate) Inverse() ExchangeRate {
	return ExchangeRate{Base: r.Quote, Quote: r.Base, Value: rateDiv(decimal.NewFromInt(1), r.Value)}
}

// String formats the rate with its direction, e.g. "100 RUB per 1 EUR".
func (r ExchangeRate) String() string {
	return fmt.Sprintf("%s %s per 1 %s", r.Value, r.Quote, r.Base)
}

// NormalizeRate converts a quote published for nominal units of base (for example "5500 KZT per 100 RUB") to a per-unit rate
// ("55 KZT per 1 RUB"). value and nominal must be positive ([ErrNonPositiveRate]); the result is validated.
func NormalizeRate(base, quote Currency, value decimal.Decimal, nominal int64) (ExchangeRate, error) {
	if nominal <= 0 || !value.IsPositive() {
		return ExchangeRate{}, fmt.Errorf("%w: value %s nominal %d", ErrNonPositiveRate, value, nominal)
	}
	r := ExchangeRate{Base: base, Quote: quote, Value: rateDiv(value, decimal.NewFromInt(nominal))}
	return r, r.Validate()
}

// CrossRate derives the rate of target per one unit of base from two quotes that share a common currency: baseQuote is "common per 1
// base" and targetQuote is "common per 1 target". The result is "target per 1 base": CrossRate(EUR/RUB = 100, USD/RUB = 80) is
// 1.25 USD per 1 EUR. Both quotes must be valid and quoted in the same currency ([ErrPairMismatch]).
func CrossRate(baseQuote, targetQuote ExchangeRate) (ExchangeRate, error) {
	if err := baseQuote.Validate(); err != nil {
		return ExchangeRate{}, err
	}
	if err := targetQuote.Validate(); err != nil {
		return ExchangeRate{}, err
	}
	if baseQuote.Quote != targetQuote.Quote {
		return ExchangeRate{}, fmt.Errorf("%w: quotes are in %s and %s, a common currency is required",
			ErrPairMismatch, baseQuote.Quote, targetQuote.Quote)
	}
	return ExchangeRate{
		Base:  baseQuote.Base,
		Quote: targetQuote.Base,
		Value: rateDiv(baseQuote.Value, targetQuote.Value),
	}, nil
}

// Convert returns m in the rate's quote currency, m × rate.Value rounded to the quote currency's minor units with mode. m must be in the
// rate's base currency ([ErrCurrencyMismatch]) and the rate valid (see [ExchangeRate.Validate]).
func (m Money) Convert(rate ExchangeRate, mode RoundingMode) (Money, error) {
	if m.currency.IsZero() {
		return Money{}, ErrInvalidCurrency
	}
	if err := rate.Validate(); err != nil {
		return Money{}, err
	}
	if m.currency != rate.Base {
		return Money{}, fmt.Errorf("%w: amount in %s, rate per 1 %s", ErrCurrencyMismatch, m.currency, rate.Base)
	}
	return Money{amount: m.amount.Mul(rate.Value), currency: rate.Quote}.Round(mode), nil
}

// rateDiv divides a by b keeping [RateDivisionPrecision] fractional digits.
func rateDiv(a, b decimal.Decimal) decimal.Decimal {
	return a.DivRound(b, RateDivisionPrecision)
}
