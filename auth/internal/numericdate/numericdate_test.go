// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package numericdate_test

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

func TestCompareOffset(t *testing.T) {
	t.Parallel()
	// A rational oracle checks both sides of the rounding midpoint across
	// zero, the epoch boundary, and a carry into the next whole second.
	values := []string{"-1.0000000009", "-0.0000000015", "-0.00000000051", "-0.0000000005", "-0.0000000001", "0",
		"0.0000000001", "0.0000000005", "0.00000000051", "0.0000000015", "0.9999999999999", "1.0000000009"}
	for _, a := range values {
		d, ok := numericdate.Parse(json.Number(a))
		require.True(t, ok)
		x, _ := new(big.Rat).SetString(a)
		for _, b := range values {
			start, valid := numericdate.Parse(json.Number(b))
			require.True(t, valid)
			y, _ := new(big.Rat).SetString(b)
			for _, offset := range []time.Duration{-time.Second, -1, 0, 1, time.Second} {
				threshold := new(big.Rat).Add(y, big.NewRat(int64(offset), int64(time.Second)))
				require.Equal(t, x.Cmp(threshold), d.CompareOffset(start, offset), "%s vs %s + %v", a, b, offset)
			}
		}
	}
}

func TestSparseExponent(t *testing.T) {
	t.Parallel()
	tiny, ok := numericdate.Parse(json.Number("1e-999999999999999999999999999999"))
	require.True(t, ok)
	smaller, ok := numericdate.Parse(json.Number("1e-1000000000000000000000000000000"))
	require.True(t, ok)
	require.Positive(t, tiny.CompareOffset(smaller, 0))
	require.Equal(t, time.Unix(0, 0).UTC(), tiny.Time())
	require.Equal(t, time.Unix(0, 1).UTC(), tiny.Expiry())
	require.True(t, numericdate.EqualNumbers(json.Number("10e-1000000000000000000000000000000"), json.Number("1e-999999999999999999999999999999")))
}

func TestNumericDateRange(t *testing.T) {
	t.Parallel()
	for _, literal := range []string{"01", "+1", "null", "NaN", "0x1p0", "1/2", "1e99999999999999999999999999", "-9223372036854775808.00000000001"} {
		_, ok := numericdate.Parse(json.Number(literal))
		require.False(t, ok, literal)
	}
	low, ok := numericdate.Parse(int64(math.MinInt64))
	require.True(t, ok)
	high, ok := numericdate.Parse(json.Number("9.223372036854775807e18"))
	require.True(t, ok)
	require.Positive(t, low.CompareOffset(low, -time.Second))
	require.Negative(t, high.CompareOffset(high, time.Second))
}
