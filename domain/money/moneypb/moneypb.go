// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package moneypb

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"

	"github.com/altessa-s/go-atlas/domain/money"

	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

const (
	// maxNanos is the largest magnitude of google.type.Money.nanos.
	maxNanos = 999_999_999
	// nanoDigits is the number of fractional digits google.type.Money holds.
	nanoDigits = 9
)

// Integer-part bounds of google.type.Money.units.
var (
	maxUnits = decimal.NewFromInt(math.MaxInt64)
	minUnits = decimal.NewFromInt(math.MinInt64)
)

// maxIntegerDigits is the number of integer digits of the largest
// google.type.Money units (math.MaxInt64 has 19).
const maxIntegerDigits = 19

// decimalBase is the base the digits of a coefficient are counted in.
const decimalBase = 10

// Bounds of an amount google.type.Money can carry: the units bounds with the
// largest nanos of the same sign.
var (
	maxAmount = maxUnits.Add(decimal.New(maxNanos, -nanoDigits))
	minAmount = minUnits.Sub(decimal.New(maxNanos, -nanoDigits))
)

// MoneyFromProto converts a google.type.Money to a money.Money of exactly
// units + nanos·10⁻⁹. A nil p is the zero Money (no currency, absent). It
// checks only the structure of google.type.Money, in this order: nanos
// outside ±999,999,999 ([ErrInvalidNanos]), units and nanos of opposite signs
// ([ErrSignMismatch]), an empty currency code ([ErrMissingCurrency]) and a
// code that is not ISO 4217 ([ErrUnknownCurrency]; matched case-insensitively
// like money.ParseCurrency). Sign and currency policy is the caller's.
func MoneyFromProto(p *gtypemoney.Money) (money.Money, error) {
	if p == nil {
		return money.Money{}, nil
	}
	units, nanos, code := p.GetUnits(), p.GetNanos(), p.GetCurrencyCode()
	switch {
	case nanos < -maxNanos || nanos > maxNanos:
		return money.Money{}, fmt.Errorf("%w: %d", ErrInvalidNanos, nanos)
	case (units > 0 && nanos < 0) || (units < 0 && nanos > 0):
		return money.Money{}, fmt.Errorf("%w: units %d, nanos %d", ErrSignMismatch, units, nanos)
	case code == "":
		return money.Money{}, ErrMissingCurrency
	}
	currency, err := money.ParseCurrency(code)
	if err != nil {
		return money.Money{}, fmt.Errorf("%w %q", ErrUnknownCurrency, code)
	}
	return money.New(decimal.NewFromInt(units).Add(decimal.New(int64(nanos), -nanoDigits)), currency)
}

// MoneyToProto converts m to a google.type.Money in m's currency, exactly:
// units is the integer part and nanos the fraction in 10⁻⁹, both with the
// amount's sign. The zero Money (no currency) is nil. An amount with more
// than nine significant fractional digits is [ErrPrecision] rather than
// silently truncated (round it first, e.g. with money.Money.Round); an
// integer part outside int64 is [ErrOverflow].
func MoneyToProto(m money.Money) (*gtypemoney.Money, error) {
	if m.Currency().IsZero() {
		return nil, nil //nolint:nilnil // the zero Money is absent, mirroring MoneyFromProto(nil)
	}
	amount := m.Amount()
	if !amount.Equal(amount.Truncate(nanoDigits)) {
		return nil, fmt.Errorf("%w: %s", ErrPrecision, m)
	}
	units := amount.Truncate(0)
	if units.GreaterThan(maxUnits) || units.LessThan(minUnits) {
		return nil, fmt.Errorf("%w: %s", ErrOverflow, m)
	}
	return &gtypemoney.Money{
		CurrencyCode: m.Currency().Code(),
		Units:        units.IntPart(),
		Nanos:        int32(amount.Sub(units).Shift(nanoDigits).IntPart()), //nolint:gosec // |nanos| < 1e9 by construction
	}, nil
}

// MoneyToProtoFitted is [MoneyToProto] for an amount that must always encode,
// such as a computed total in a response: digits beyond the ninth decimal are
// truncated toward zero and an amount beyond the range of google.type.Money
// saturates at its bound. The zero Money (no currency) is nil. A fitted amount
// always encodes, so a failure is a programming error and panics rather than
// dropping the amount from a response.
func MoneyToProtoFitted(m money.Money) *gtypemoney.Money {
	if m.Currency().IsZero() {
		return nil
	}
	fitted, err := money.New(fitAmount(m.Amount()), m.Currency())
	if err != nil {
		panic(fmt.Sprintf("moneypb: fitted amount does not form a Money: %v", err))
	}
	p, err := MoneyToProto(fitted)
	if err != nil {
		panic(fmt.Sprintf("moneypb: fitted amount does not encode: %v", err))
	}
	return p
}

// fitAmount truncates d to nine decimals toward zero and saturates it at the
// range of google.type.Money. The magnitude is classified first with integer
// arithmetic on the coefficient's digit count and the exponent: rescaling a
// compact amount such as 1e1000000000 or 1e-1000000000 directly would build
// an enormous power of ten. Below 10⁻⁹ the result is zero, from 10¹⁹ on it is
// the bound of d's sign; in between the exponent is bounded, so Truncate and
// the comparisons rescale by at most a few dozen digits.
func fitAmount(d decimal.Decimal) decimal.Decimal {
	if d.IsZero() {
		return decimal.Zero
	}
	// |d| < 10^integerDigits and |d| ≥ 10^(integerDigits-1). The digit count
	// is exact: decimal.NumDigits estimates small coefficients with a float
	// logarithm and undercounts some powers of ten (shopspring/decimal#420).
	coefficient := d.Coefficient()
	integerDigits := int64(len(coefficient.Abs(coefficient).Text(decimalBase))) + int64(d.Exponent())
	switch {
	case integerDigits <= -nanoDigits:
		return decimal.Zero
	case integerDigits > maxIntegerDigits:
		if d.IsNegative() {
			return minAmount
		}
		return maxAmount
	}
	return decimal.Min(decimal.Max(d.Truncate(nanoDigits), minAmount), maxAmount)
}
