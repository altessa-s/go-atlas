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

func TestAllocateHalfUpLastRemainder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		amount string
		ratios []string
		want   []string
	}{
		{"last_takes_remainder", "100.00", []string{"1", "1", "1"}, []string{"33.33 EUR", "33.33 EUR", "33.34 EUR"}},
		{"half_up", "0.05", []string{"1", "1"}, []string{"0.03 EUR", "0.02 EUR"}},
		{"small_shares_round_down", "0.01", []string{"1", "1", "1"}, []string{"0.00 EUR", "0.00 EUR", "0.01 EUR"}},
		// Five half-unit shares round up to 0.05, past the 0.03 total: two
		// units come back from the latest of the equally rounded-up shares.
		{"units_taken_back", "0.03", []string{"1", "1", "1", "1", "1", "1"},
			[]string{"0.01 EUR", "0.01 EUR", "0.01 EUR", "0.00 EUR", "0.00 EUR", "0.00 EUR"}},
		// 0.015 rounds up to 0.02 and 0.0125 down to 0.01; the last share
		// takes the 0.01 left, no unit comes back.
		{"mixed_rounding", "0.04", []string{"6", "5", "5"}, []string{"0.02 EUR", "0.01 EUR", "0.01 EUR"}},
		{"negative", "-100.00", []string{"1", "1", "1"}, []string{"-33.33 EUR", "-33.33 EUR", "-33.34 EUR"}},
		{"negative_small", "-0.01", []string{"1", "1", "1"}, []string{"0.00 EUR", "0.00 EUR", "-0.01 EUR"}},
		// The exact share is rounded to 28 places first: 0.00499…(28 places)
		// becomes 0.005 and rounds up.
		{"intermediate_precision", "0.01", []string{"1", "1.000000000000000000000000001"}, []string{"0.01 EUR", "0.00 EUR"}},
		{"single", "7.77", []string{"3"}, []string{"7.77 EUR"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ratios := make([]decimal.Decimal, len(tc.ratios))
			for i, r := range tc.ratios {
				ratios[i] = dec(r)
			}
			m := mustParse(t, tc.amount, "EUR")
			parts, err := m.AllocateBy(money.HalfUpLastRemainder, ratios...)
			require.NoError(t, err)
			require.Equal(t, tc.want, strings(parts))
			requireSum(t, m, parts)
		})
	}
}

func TestAllocateByErrors(t *testing.T) {
	t.Parallel()

	m := mustParse(t, "1.00", "EUR")
	_, err := m.AllocateBy(money.AllocationMethod(9), dec("1"))
	require.Error(t, err)
	_, err = m.AllocateBy(money.HalfUpLastRemainder)
	require.ErrorIs(t, err, money.ErrInvalidRatios)
	require.Equal(t, "half-up-last-remainder", money.HalfUpLastRemainder.String())
	require.Equal(t, "largest-remainder", money.LargestRemainder.String())
	require.Equal(t, "AllocationMethod(9)", money.AllocationMethod(9).String())
}

// referenceAllocate is the SubLedger specification §7.5 implementation the
// HalfUpLastRemainder method must reproduce (subledger
// internal/pkg/money/allocate.go), kept verbatim as a test oracle.
func referenceAllocate(total decimal.Decimal, weights []decimal.Decimal, places int32) []decimal.Decimal {
	sum := decimal.Zero
	for _, w := range weights {
		sum = sum.Add(w)
	}
	exact := make([]decimal.Decimal, len(weights))
	for i, w := range weights {
		exact[i] = total.Mul(w).DivRound(sum, 28)
	}
	n := len(exact)
	shares := make([]decimal.Decimal, n)
	allocated := decimal.Zero
	for i := range n - 1 {
		shares[i] = exact[i].Round(places)
		allocated = allocated.Add(shares[i])
	}
	shares[n-1] = total.Sub(allocated)
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
			break
		}
		shares[best] = shares[best].Sub(unit)
		shares[n-1] = shares[n-1].Add(unit)
	}
	return shares
}

// FuzzHalfUpLastRemainderMatchesReference pins that the method reproduces
// the specification's apportioning exactly, share by share.
func FuzzHalfUpLastRemainderMatchesReference(f *testing.F) {
	f.Add(int32(3), uint16(1), uint16(1), uint16(1), uint16(1), int8(0))
	f.Add(int32(10000), uint16(1), uint16(2), uint16(3), uint16(0), int8(2))
	f.Add(int32(-2), uint16(1), uint16(1), uint16(1), uint16(0), int8(0))
	f.Add(int32(2), uint16(6), uint16(6), uint16(6), uint16(2), int8(28))
	f.Fuzz(func(t *testing.T, minor int32, w1, w2, w3, w4 uint16, scale int8) {
		// The convention is defined for non-negative amounts; a negative one
		// is checked as the mirror of its absolute value.
		negative := minor < 0
		if negative {
			minor = -(minor + 1)
		}
		if w1|w2|w3|w4 == 0 {
			return
		}
		// Decimal weights at any scale, so intermediate rounding matters.
		exp := -int32(scale % 30)
		weights := []decimal.Decimal{
			decimal.New(int64(w1), exp), decimal.New(int64(w2), exp), decimal.New(int64(w3)+1, exp-1), decimal.New(int64(w4), exp),
		}
		m, err := money.FromMinor(int64(minor), eur)
		require.NoError(t, err)
		parts, err := m.AllocateBy(money.HalfUpLastRemainder, weights...)
		require.NoError(t, err)
		want := referenceAllocate(m.Amount(), weights, eur.MinorUnits())
		for i := range parts {
			require.True(t, want[i].Equal(parts[i].Amount()), "share %d: got %s, want %s (weights %v)", i, parts[i], want[i], weights)
			require.False(t, parts[i].IsNegative(), "share %d is negative", i)
		}
		requireSum(t, m, parts)

		if negative {
			neg, err := m.Neg().AllocateBy(money.HalfUpLastRemainder, weights...)
			require.NoError(t, err)
			for i := range neg {
				require.True(t, neg[i].Equal(parts[i].Neg()), "negative share %d mirrors the positive one", i)
			}
		}
	})
}
