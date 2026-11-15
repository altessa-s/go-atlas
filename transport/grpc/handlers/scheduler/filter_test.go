// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/filter"
)

func TestScalar_int64Bounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      any
		want    any
		wantErr bool
	}{
		{name: "float lower bound", in: float64(math.MinInt64), want: int64(math.MinInt64)},
		{name: "float below lower bound", in: math.Nextafter(math.MinInt64, math.Inf(-1)), wantErr: true},
		{name: "float largest below 2^63", in: math.Nextafter(0x1p63, 0), want: int64(math.Nextafter(0x1p63, 0))},
		{name: "float 2^63 overflows", in: float64(0x1p63), wantErr: true},
		{name: "float fraction", in: 1.5, wantErr: true},
		{name: "uint64 max int64", in: uint64(math.MaxInt64), want: int64(math.MaxInt64)},
		{name: "uint64 overflows", in: uint64(math.MaxInt64) + 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := scalar(tc.in)
			if tc.wantErr {
				require.ErrorIs(t, err, filter.ErrUnsupportedType)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
