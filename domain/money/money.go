// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/shopspring/decimal"
)

var (
	// ErrUnknownCurrency is returned by [ParseCurrency] for a code that is
	// not an active ISO 4217 currency.
	ErrUnknownCurrency = errors.New("money: unknown currency")
	// ErrInvalidCurrency is returned by [NewCurrency] for an empty code or
	// out-of-range minor units, and by operations on a zero Currency.
	ErrInvalidCurrency = errors.New("money: invalid currency")
	// ErrCurrencyMismatch is returned when two amounts of different
	// currencies are combined or compared.
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	// ErrNotRounded is returned when an operation needs an amount in whole
	// minor units and the amount has more decimal places.
	ErrNotRounded = errors.New("money: amount has more decimal places than its currency")
	// ErrOverflow is returned when an amount in minor units does not fit in
	// an int64.
	ErrOverflow = errors.New("money: amount out of range")
	// ErrInvalidRatios is returned by [Money.Allocate] for no ratios, a
	// negative ratio, or ratios summing to zero.
	ErrInvalidRatios = errors.New("money: invalid allocation ratios")
)

// Money is an amount of a currency. It is an immutable value: operations
// return new values and never round implicitly — arithmetic keeps every
// decimal place, and [Money.Round] rounds to the currency's minor units
// with an explicit mode. The zero Money has no currency and is invalid for
// every operation but [Money.IsZero] and [Money.Currency].
type Money struct {
	amount   decimal.Decimal
	currency Currency
}

// New returns amount in currency.
func New(amount decimal.Decimal, currency Currency) (Money, error) {
	if currency.IsZero() {
		return Money{}, ErrInvalidCurrency
	}
	return Money{amount: amount, currency: currency}, nil
}

// Parse returns the amount written in decimal notation ("12.30", "-0.5")
// in the ISO 4217 currency code.
func Parse(amount, code string) (Money, error) {
	currency, err := ParseCurrency(code)
	if err != nil {
		return Money{}, err
	}
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return Money{}, fmt.Errorf("money: parse amount %q: %w", amount, err)
	}
	return Money{amount: d, currency: currency}, nil
}

// FromMinor returns the amount given in minor units: FromMinor(1230, EUR)
// is 12.30 EUR.
func FromMinor(minor int64, currency Currency) (Money, error) {
	if currency.IsZero() {
		return Money{}, ErrInvalidCurrency
	}
	return Money{amount: decimal.New(minor, -currency.minorUnits), currency: currency}, nil
}

// Zero returns zero in currency.
func Zero(currency Currency) Money { return Money{amount: decimal.Zero, currency: currency} }

// Amount returns the amount.
func (m Money) Amount() decimal.Decimal { return m.amount }

// Currency returns the currency.
func (m Money) Currency() Currency { return m.currency }

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool { return m.amount.IsZero() }

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m.amount.IsNegative() }

// IsPositive reports whether the amount is above zero.
func (m Money) IsPositive() bool { return m.amount.IsPositive() }

// sameCurrency fails unless m and o share a valid currency.
func (m Money) sameCurrency(o Money) error {
	if m.currency.IsZero() || o.currency.IsZero() {
		return ErrInvalidCurrency
	}
	if m.currency != o.currency {
		return fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return nil
}

// Add returns m + o.
func (m Money) Add(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Add(o.amount), currency: m.currency}, nil
}

// Sub returns m - o.
func (m Money) Sub(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Sub(o.amount), currency: m.currency}, nil
}

// Neg returns -m.
func (m Money) Neg() Money { return Money{amount: m.amount.Neg(), currency: m.currency} }

// Abs returns |m|.
func (m Money) Abs() Money { return Money{amount: m.amount.Abs(), currency: m.currency} }

// Mul returns m multiplied by factor, exactly: the product keeps every
// decimal place. Round it to apply a rate, a tax or a discount.
func (m Money) Mul(factor decimal.Decimal) Money {
	return Money{amount: m.amount.Mul(factor), currency: m.currency}
}

// Cmp compares m and o: -1, 0 or +1.
func (m Money) Cmp(o Money) (int, error) {
	if err := m.sameCurrency(o); err != nil {
		return 0, err
	}
	return m.amount.Cmp(o.amount), nil
}

// Equal reports whether m and o are the same amount of the same currency;
// 1.5 and 1.50 are equal.
func (m Money) Equal(o Money) bool {
	return m.currency == o.currency && m.amount.Equal(o.amount)
}

// RoundingMode selects how [Money.Round] treats the discarded digits.
type RoundingMode uint8

const (
	// HalfEven rounds to the nearest value, a tie to the even neighbor
	// (banker's rounding): 0.125 → 0.12, 0.135 → 0.14.
	HalfEven RoundingMode = iota
	// HalfUp rounds to the nearest value, a tie away from zero: 0.125 →
	// 0.13, -0.125 → -0.13.
	HalfUp
	// Down rounds toward zero (truncates).
	Down
	// Up rounds away from zero.
	Up
	// Floor rounds toward negative infinity.
	Floor
	// Ceiling rounds toward positive infinity.
	Ceiling
)

// String returns the mode name.
func (r RoundingMode) String() string {
	switch r {
	case HalfEven:
		return "half-even"
	case HalfUp:
		return "half-up"
	case Down:
		return "down"
	case Up:
		return "up"
	case Floor:
		return "floor"
	case Ceiling:
		return "ceiling"
	default:
		return fmt.Sprintf("RoundingMode(%d)", uint8(r))
	}
}

// Round returns m rounded to its currency's minor units with mode.
func (m Money) Round(mode RoundingMode) Money {
	places := m.currency.minorUnits
	var d decimal.Decimal
	switch mode {
	case HalfUp:
		d = m.amount.Round(places)
	case Down:
		d = m.amount.RoundDown(places)
	case Up:
		d = m.amount.RoundUp(places)
	case Floor:
		d = m.amount.RoundFloor(places)
	case Ceiling:
		d = m.amount.RoundCeil(places)
	default:
		d = m.amount.RoundBank(places)
	}
	return Money{amount: d, currency: m.currency}
}

// IsRounded reports whether m has no more decimal places than its currency.
func (m Money) IsRounded() bool {
	return m.amount.Equal(m.amount.Truncate(m.currency.minorUnits))
}

// MinorUnits returns the amount in minor units: 12.30 EUR is 1230. It fails
// with [ErrNotRounded] for an amount with more decimal places than the
// currency and [ErrOverflow] beyond int64.
func (m Money) MinorUnits() (int64, error) {
	minor, err := m.minorBig()
	if err != nil {
		return 0, err
	}
	if !minor.IsInt64() {
		return 0, fmt.Errorf("%w: %s", ErrOverflow, minor)
	}
	return minor.Int64(), nil
}

// maxMinorDigits bounds the decimal digits of an amount in minor units that
// [Money.minorBig] materializes; int64 holds 19.
const maxMinorDigits = 19

// minorBig returns the amount in minor units as a big integer, failing with
// [ErrOverflow] beyond maxMinorDigits digits before building it, so an
// amount like 1e1000000000 cannot exhaust memory.
func (m Money) minorBig() (*big.Int, error) {
	if m.currency.IsZero() {
		return nil, ErrInvalidCurrency
	}
	if m.amount.IsZero() {
		return new(big.Int), nil
	}
	// Exponent is int32, minor units at most MaxMinorUnits: no overflow in
	// int64.
	if digits := int64(m.amount.NumDigits()) + int64(m.amount.Exponent()) + int64(m.currency.minorUnits); digits > maxMinorDigits {
		return nil, fmt.Errorf("%w: about %d digits in minor units", ErrOverflow, digits)
	}
	if !m.IsRounded() {
		return nil, fmt.Errorf("%w: %d decimal places", ErrNotRounded, -m.amount.Exponent())
	}
	return m.amount.Shift(m.currency.minorUnits).BigInt(), nil
}

// String returns the amount with exactly the currency's minor units — more
// when the amount is not rounded — and the code: "12.30 EUR".
func (m Money) String() string {
	places := max(m.currency.minorUnits, -m.amount.Exponent())
	return m.amount.StringFixed(places) + " " + m.currency.code
}

// AllocationMethod selects how [Money.AllocateBy] rounds the shares to
// minor units while keeping their sum equal to the amount.
type AllocationMethod uint8

const (
	// LargestRemainder rounds every share toward zero and hands the minor
	// units left over one each to the shares with the largest discarded
	// remainders, ties to the earlier share. No share differs from its
	// exact value by a minor unit or more.
	LargestRemainder AllocationMethod = iota
	// HalfUpLastRemainder rounds every share but the last half-up (away from
	// zero) and gives the last share the remainder. When the rounded shares
	// would push the last one below zero, the missing units are taken back
	// one at a time from the earlier shares rounded up the most over their
	// exact value, ties to the later share. Exact shares are computed as
	// amount*ratio/sum rounded to [HalfUpLastRemainderPrecision] decimal
	// places, so the result matches implementations of this convention on
	// shopspring decimals share for share. The convention is defined for
	// non-negative amounts; a negative amount is split as its absolute value
	// and every part negated.
	HalfUpLastRemainder
)

// String returns the method name.
func (a AllocationMethod) String() string {
	switch a {
	case LargestRemainder:
		return "largest-remainder"
	case HalfUpLastRemainder:
		return "half-up-last-remainder"
	default:
		return fmt.Sprintf("AllocationMethod(%d)", uint8(a))
	}
}

// Allocate splits m into parts proportional to ratios with the
// [LargestRemainder] method. See [Money.AllocateBy].
func (m Money) Allocate(ratios ...decimal.Decimal) ([]Money, error) {
	return m.AllocateBy(LargestRemainder, ratios...)
}

// HalfUpLastRemainderPrecision is the number of decimal places of the exact
// shares [HalfUpLastRemainder] rounds.
const HalfUpLastRemainderPrecision = 28

// AllocateBy splits m into parts proportional to ratios without losing or
// creating a minor unit: the parts sum to m exactly, rounded to minor units
// by method. m must be in whole minor units ([ErrNotRounded]) that fit in an
// int64 ([ErrOverflow]); ratios must be non-negative and not all zero
// ([ErrInvalidRatios]). [LargestRemainder] computes exact shares with
// rational arithmetic and splits a negative m as its absolute value, every
// part negated; [HalfUpLastRemainder] is defined on decimals (see there).
func (m Money) AllocateBy(method AllocationMethod, ratios ...decimal.Decimal) ([]Money, error) {
	total, err := m.minorBig()
	if err != nil {
		return nil, err
	}
	if len(ratios) == 0 {
		return nil, fmt.Errorf("%w: none given", ErrInvalidRatios)
	}
	sum := new(big.Rat)
	rats := make([]*big.Rat, len(ratios))
	for i, r := range ratios {
		if r.IsNegative() {
			return nil, fmt.Errorf("%w: ratio %d is negative", ErrInvalidRatios, i)
		}
		rats[i] = r.Rat()
		sum.Add(sum, rats[i])
	}
	if sum.Sign() == 0 {
		return nil, fmt.Errorf("%w: ratios sum to zero", ErrInvalidRatios)
	}

	switch method {
	case LargestRemainder:
	case HalfUpLastRemainder:
		return m.halfUpLastRemainder(ratios), nil
	default:
		return nil, fmt.Errorf("money: unknown allocation method %d", uint8(method))
	}

	negative := total.Sign() < 0
	total.Abs(total)
	totalRat := new(big.Rat).SetInt(total)
	exact := make([]*big.Rat, len(rats))
	for i, r := range rats {
		exact[i] = new(big.Rat).Mul(totalRat, r)
		exact[i].Quo(exact[i], sum)
	}
	parts := largestRemainder(total, exact)

	out := make([]Money, len(parts))
	for i, p := range parts {
		if negative {
			p.Neg(p)
		}
		out[i] = Money{amount: decimal.NewFromBigInt(p, -m.currency.minorUnits), currency: m.currency}
	}
	return out, nil
}

// floorRat returns the floor of the non-negative r.
func floorRat(r *big.Rat) *big.Int {
	return new(big.Int).Quo(r.Num(), r.Denom())
}

// largestRemainder implements [LargestRemainder] on non-negative exact
// shares of total, in minor units.
func largestRemainder(total *big.Int, exact []*big.Rat) []*big.Int {
	type share struct {
		index     int
		remainder *big.Rat
	}
	parts := make([]*big.Int, len(exact))
	shares := make([]share, len(exact))
	allocated := new(big.Int)
	for i, e := range exact {
		parts[i] = floorRat(e)
		allocated.Add(allocated, parts[i])
		shares[i] = share{index: i, remainder: new(big.Rat).Sub(e, new(big.Rat).SetInt(parts[i]))}
	}
	// The stable sort keeps the earlier share first among equal remainders.
	slices.SortStableFunc(shares, func(a, b share) int { return b.remainder.Cmp(a.remainder) })
	left := new(big.Int).Sub(total, allocated).Int64() // < len(exact)
	for i := range left {
		parts[shares[i].index].Add(parts[shares[i].index], big.NewInt(1))
	}
	return parts
}

// halfUpLastRemainder implements [HalfUpLastRemainder] on validated ratios.
func (m Money) halfUpLastRemainder(ratios []decimal.Decimal) []Money {
	if m.amount.IsNegative() {
		parts := m.Neg().halfUpLastRemainder(ratios)
		for i := range parts {
			parts[i] = parts[i].Neg()
		}
		return parts
	}
	places := m.currency.minorUnits
	sum := decimal.Zero
	for _, r := range ratios {
		sum = sum.Add(r)
	}
	n := len(ratios)
	exact := make([]decimal.Decimal, n)
	for i, r := range ratios {
		exact[i] = m.amount.Mul(r).DivRound(sum, HalfUpLastRemainderPrecision)
	}
	shares := make([]decimal.Decimal, n)
	allocated := decimal.Zero
	for i := range n - 1 {
		shares[i] = exact[i].Round(places)
		allocated = allocated.Add(shares[i])
	}
	shares[n-1] = m.amount.Sub(allocated)

	unit := decimal.New(1, -places)
	for shares[n-1].IsNegative() {
		best := -1
		bestExcess := decimal.Zero
		for i := range n - 1 {
			excess := shares[i].Sub(exact[i])
			if excess.IsPositive() && (best < 0 || excess.GreaterThanOrEqual(bestExcess)) {
				best, bestExcess = i, excess
			}
		}
		if best < 0 {
			// Unreachable for exact shares of a non-negative amount: a
			// negative remainder means some earlier share was rounded up.
			break
		}
		shares[best] = shares[best].Sub(unit)
		shares[n-1] = shares[n-1].Add(unit)
	}

	out := make([]Money, n)
	for i, sh := range shares {
		out[i] = Money{amount: sh, currency: m.currency}
	}
	return out
}

// Split divides m into n parts as equal as possible with the
// [LargestRemainder] method: Split(3) of 10.00 is 3.34, 3.33, 3.33. See
// [Money.AllocateBy].
func (m Money) Split(n int) ([]Money, error) {
	if n <= 0 {
		return nil, fmt.Errorf("%w: %d parts", ErrInvalidRatios, n)
	}
	ratios := make([]decimal.Decimal, n)
	for i := range ratios {
		ratios[i] = decimal.NewFromInt(1)
	}
	return m.Allocate(ratios...)
}
