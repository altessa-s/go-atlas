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

func TestAllocateRejectsInt64Overflow(t *testing.T) {
	t.Parallel()
	for _, amount := range []string{"92233720368547758.08", "-92233720368547758.09"} {
		t.Run(amount, func(t *testing.T) {
			t.Parallel()
			m, err := money.Parse(amount, "EUR")
			require.NoError(t, err)
			for _, method := range []money.AllocationMethod{money.LargestRemainder, money.HalfUpLastRemainder} {
				_, err := m.AllocateBy(method, decimal.NewFromInt(1))
				require.ErrorIs(t, err, money.ErrOverflow)
			}
		})
	}
}
