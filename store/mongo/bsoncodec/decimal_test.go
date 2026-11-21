// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"math/big"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"

	mongoxcodec "github.com/altessa-s/go-mongox/bsoncodec"
)

func TestDecimalCoefficientLimit(t *testing.T) {
	t.Parallel()
	// Both coefficients represent small integers after the trailing zeros fold.
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(1232), nil)
	for _, tc := range []struct {
		name   string
		factor int64
		bits   int
	}{
		{"at limit", 10, 4096},
		{"over limit", 11, 4097},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			coef := new(big.Int).Mul(base, big.NewInt(tc.factor))
			require.Equal(t, tc.bits, coef.BitLen())
			raw, err := marshal(bson.D{{Key: "v", Value: decimal.NewFromBigInt(coef, -1232)}})
			if tc.bits > 4096 {
				require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange)
				require.ErrorIs(t, err, mongoxcodec.ErrDecimalOutOfRange)
				return
			}
			require.NoError(t, err)
			var out struct{ V decimal.Decimal }
			require.NoError(t, unmarshal(raw, &out))
			require.True(t, decimal.NewFromInt(tc.factor).Equal(out.V))
		})
	}
}

func TestNonFiniteDecimalError(t *testing.T) {
	t.Parallel()
	nan, err := bson.ParseDecimal128("NaN")
	require.NoError(t, err)
	raw, err := bson.Marshal(bson.D{{Key: "v", Value: nan}})
	require.NoError(t, err)
	var out struct{ V decimal.Decimal }
	err = unmarshal(raw, &out)
	require.ErrorIs(t, err, bsoncodec.ErrDecimalNotFinite)
	require.ErrorIs(t, err, mongoxcodec.ErrDecimalNotFinite)
	var decodeErr *bson.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	require.Equal(t, []string{"v"}, decodeErr.Keys())
}
