// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseSortString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bson.D
	}{
		{"single asc", "name", bson.D{{Key: "name", Value: SortAscending}}},
		{"single desc", "-name", bson.D{{Key: "name", Value: SortDescending}}},
		{"multiple", "name,-age", bson.D{
			{Key: "name", Value: SortAscending},
			{Key: "age", Value: SortDescending},
		}},
		{"empty", "", nil},
		{"whitespace only", "  ,  ", nil},
		{"dash only", "-", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSortString(tt.input)
			require.Len(t, got, len(tt.want))
			for i := range got {
				require.Equal(t, tt.want[i].Key, got[i].Key, "[%d] key mismatch", i)
				require.Equal(t, tt.want[i].Value, got[i].Value, "[%d] value mismatch", i)
			}
		})
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{5, 8},
		{8, 8},
		{100, 128},
		{-1, 1},
	}

	for _, tt := range tests {
		got := nextPowerOfTwo(tt.n)
		require.Equal(t, tt.want, got, "nextPowerOfTwo(%d)", tt.n)
	}
}

func TestCapListLimit(t *testing.T) {
	logger := slog.Default()

	require.Equal(t, int64(500), capListLimit(500, logger))
	require.Equal(t, int64(MaxListLimit), capListLimit(2000, logger))
}

func TestParseSortOption(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		d, ok := parseSortOption("name,-age")
		require.True(t, ok, "expected ok")
		require.Len(t, d, 2)
	})

	t.Run("empty string", func(t *testing.T) {
		_, ok := parseSortOption("")
		require.False(t, ok, "expected not ok for empty string")
	})

	t.Run("*string", func(t *testing.T) {
		s := "name"
		d, ok := parseSortOption(&s)
		require.True(t, ok, "expected ok")
		require.Len(t, d, 1)
	})

	t.Run("nil *string", func(t *testing.T) {
		_, ok := parseSortOption[*string](nil)
		require.False(t, ok, "expected not ok for nil pointer")
	})

	t.Run("bson.D", func(t *testing.T) {
		input := bson.D{{Key: "name", Value: 1}}
		d, ok := parseSortOption(input)
		require.True(t, ok, "expected ok")
		require.Equal(t, "name", d[0].Key)
	})

	t.Run("empty bson.D", func(t *testing.T) {
		_, ok := parseSortOption(bson.D{})
		require.False(t, ok, "expected not ok for empty bson.D")
	})
}
