// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgrammaticNumericConditions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		actual, expected any
		want             bool
	}{
		{float64(1e9), int32(1000000000), true},
		{json.Number("9007199254740993"), uint64(9007199254740993), true},
		{json.Number("9007199254740993"), int64(9007199254740992), false},
		{"42", int8(42), false},
		{json.Number("1750000000.000000001"), float64(1750000000), false},
		{json.Number("0.1"), float32(0.1), true},
	} {
		require.Equal(t, tc.want, conditionValueEquals(tc.actual, tc.expected))
	}
}

func TestExpectedNumberArray(t *testing.T) {
	t.Parallel()
	numbers := []any{json.Number("1750000000.000000002")}
	require.True(t, claimMatches(numbers, "1750000000.000000002"))
	require.False(t, claimMatches(numbers, json.Number("1750000000.000000001")))
	require.False(t, claimMatches(numbers, "1750000000.000000001"))
}
