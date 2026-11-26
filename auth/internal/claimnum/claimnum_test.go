// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimnum_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/internal/claimnum"
)

func TestNumber(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		literal string
		want    any
	}{
		{"3", float64(3)},
		{"-12", float64(-12)},
		{"0.1", 0.1},
		{"1.0", float64(1)},
		{"1e3", float64(1000)},
		{"9007199254740992", float64(9007199254740992)},
		{"9007199254740993", json.Number("9007199254740993")},
		{"0.30000000000000004123", json.Number("0.30000000000000004123")},
	} {
		got, err := claimnum.Number(json.Number(tc.literal))
		require.NoError(t, err, tc.literal)
		require.Equal(t, tc.want, got, tc.literal)
	}

	_, err := claimnum.Number("1e400")
	require.ErrorIs(t, err, claimnum.ErrOutOfRange)
}

func TestDecodeNormalizesNested(t *testing.T) {
	t.Parallel()

	var got map[string]any
	require.NoError(t, claimnum.Decode([]byte(`{"a":1,"b":{"c":[2,9007199254740993]}}`), &got))
	require.Equal(t, map[string]any{
		"a": float64(1),
		"b": map[string]any{"c": []any{float64(2), json.Number("9007199254740993")}},
	}, got)

	require.Error(t, claimnum.Decode([]byte(`{"a":1e400}`), &got))
}
