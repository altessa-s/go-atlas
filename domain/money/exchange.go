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
	// ErrRateOutOfRange indicates a rate outside the supported magnitude or scale.
	ErrRateOutOfRange = errors.New("money: exchange rate out of range")
)

// RateDivisionPrecision is the number of fractional digits kept by the divisions of exchange-rate arithmetic ([NormalizeRate],
// [ExchangeRate.Inverse], [CrossRate]), so repeating fractions terminate (rounded half away from zero) without distorting rates stored
// with a dozen digits.
const RateDivisionPrecision int32 = 28

// MaxRateExponent bounds exchange rate magnitudes to [10^-MaxRateExponent,
// 10^MaxRateExponent]. This uses the default decimal decoder limit, independent
// of process-wide decimal settings.
const MaxRateExponent int32 = 10_000

// A coefficient may include fractional digits beyond the magnitude bound.
const maxRateScale = MaxRateExponent + RateDivisionPrecision

// ExchangeRate is the price of one unit of Base expressed in Quote: Value units of Quote per 1 Base. It is a plain value; [ExchangeRate.Validate]
// checks it.
type ExchangeRate struct {
	Base  Currency
	Quote Currency
	Value decimal.Decimal
}

// Validate checks that the rate names two distinct, set currencies, is
// strictly positive, and stays within the magnitude and scale limits.
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
	return validateRateAmount(r.Value, MaxRateExponent)
}

func validateRateAmount(value decimal.Decimal, maxExponent int32) error {
	if !value.IsPositive() {
		return ErrNonPositiveRate
	}
	exp := value.Exponent()
	if exp < -maxRateScale || exp > maxExponent {
		return fmt.Errorf("%w: decimal exponent %d", ErrRateOutOfRange, exp)
	}
	magnitude := int64(value.NumDigits()) - 1 + int64(exp)
	if magnitude < -int64(MaxRateExponent) || magnitude > int64(maxExponent) ||
		(magnitude == int64(maxExponent) && value.GreaterThan(decimal.New(1, maxExponent))) {
		return fmt.Errorf("%w: magnitude must be between 10^-%d and 10^%d", ErrRateOutOfRange, MaxRateExponent, maxExponent)
	}
	return nil
}

// Inverse returns the reciprocal rate. It panics for an invalid rate.
// Use [ExchangeRate.InverseChecked] when handling unvalidated input.
func (r ExchangeRate) Inverse() ExchangeRate {
	inverse, err := r.InverseChecked()
	if err != nil {
		panic(err)
	}
	return inverse
}

// InverseChecked validates the rate and returns its reciprocal with
// [RateDivisionPrecision] fractional digits. When that precision would round
// to zero, it preserves additional fractional digits up to [MaxRateExponent].
func (r ExchangeRate) InverseChecked() (ExchangeRate, error) {
	if err := r.Validate(); err != nil {
		return ExchangeRate{}, err
	}
	one := decimal.NewFromInt(1)
	value := rateDiv(one, r.Value)
	if value.IsZero() {
		digits := int64(r.Value.NumDigits()) + int64(r.Value.Exponent())
		precision := min(digits+int64(RateDivisionPrecision)-1, int64(MaxRateExponent))
		value = one.DivRound(r.Value, int32(precision)) //nolint:gosec // precision is positive and bounded by MaxRateExponent.
	}
	return ExchangeRate{Base: r.Quote, Quote: r.Base, Value: value}, nil
}

// String formats the rate with its direction, e.g. "100 RUB per 1 EUR".
func (r ExchangeRate) String() string {
	return fmt.Sprintf("%s %s per 1 %s", r.Value, r.Quote, r.Base)
}

// NormalizeRate converts a quote published for nominal units of base (for example "5500 KZT per 100 RUB") to a per-unit rate
// ("55 KZT per 1 RUB"). value and nominal must be positive ([ErrNonPositiveRate]); the result is validated.
func NormalizeRate(base, quote Currency, value decimal.Decimal, nominal int64) (ExchangeRate, error) {
	if nominal <= 0 {
		return ExchangeRate{}, fmt.Errorf("%w: nominal %d", ErrNonPositiveRate, nominal)
	}
	// A positive int64 nominal can add up to 19 decimal digits to the total
	// quote. Bound the division here and validate the per-unit rate below.
	const maxQuoteExponent = MaxRateExponent + 19
	if err := validateRateAmount(value, maxQuoteExponent); err != nil {
		return ExchangeRate{}, err
	}
	r := ExchangeRate{Base: base, Quote: quote, Value: rateDiv(value, decimal.NewFromInt(nominal))}
	return r, r.Validate()
}

// CrossRate derives the rate of target per one unit of base from two quotes that share a common currency: baseQuote is "common per 1
// base" and targetQuote is "common per 1 target". The result is "target per 1 base": CrossRate(EUR/RUB = 100, USD/RUB = 80) is
// 1.25 USD per 1 EUR. The quotes must share a quote currency and have distinct bases ([ErrPairMismatch]).
// A result that rounds to zero at [RateDivisionPrecision] returns [ErrNonPositiveRate].
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
	cross := ExchangeRate{
		Base:  baseQuote.Base,
		Quote: targetQuote.Base,
		Value: rateDiv(baseQuote.Value, targetQuote.Value),
	}
	return cross, cross.Validate()
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
