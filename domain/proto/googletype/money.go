// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype

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
