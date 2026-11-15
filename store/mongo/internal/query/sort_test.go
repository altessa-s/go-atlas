// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseSortString(t *testing.T) {
	t.Parallel()
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
		// Operator keys (`$natural`, `$id`, ...) force a full collection
		// scan and must never come from caller-supplied sort strings —
		// ParseSortString silently drops them.
		{"drops $natural", "$natural", nil},
		{"drops $natural desc", "-$natural", nil},
		{"keeps non-operator next to dropped $-key", "name,$natural", bson.D{
			{Key: "name", Value: SortAscending},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseSortString(tt.input)
			require.Len(t, got, len(tt.want))
			for i := range got {
				require.Equal(t, tt.want[i].Key, got[i].Key, "[%d] key mismatch", i)
				require.Equal(t, tt.want[i].Value, got[i].Value, "[%d] value mismatch", i)
			}
		})
	}
}

func TestParseSortStringStrict(t *testing.T) {
	t.Parallel()
	allowed := []string{"name", "createdAt", "priority"}

	t.Run("all allowed fields pass", func(t *testing.T) {
		t.Parallel()
		got, err := ParseSortStringStrict("name,-priority,createdAt", allowed)
		require.NoError(t, err)
		require.Equal(t, bson.D{
			{Key: "name", Value: SortAscending},
			{Key: "priority", Value: SortDescending},
			{Key: "createdAt", Value: SortAscending},
		}, got)
	})

	t.Run("non-allowed field rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseSortStringStrict("name,passwordHash", allowed)
		require.ErrorIs(t, err, ErrSortFieldNotAllowed)
	})

	t.Run("operator key rejected even before allowlist check", func(t *testing.T) {
		t.Parallel()
		// $natural is rejected even if a caller mistakenly puts it in the
		// allowlist — operator keys are reserved.
		_, err := ParseSortStringStrict("$natural", append(allowed, "$natural"))
		require.ErrorIs(t, err, ErrSortFieldNotAllowed)
	})

	t.Run("descending operator key also rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseSortStringStrict("-$natural", allowed)
		require.ErrorIs(t, err, ErrSortFieldNotAllowed)
	})

	t.Run("empty input returns nil without error", func(t *testing.T) {
		t.Parallel()
		got, err := ParseSortStringStrict("", allowed)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("empty allowlist rejects everything", func(t *testing.T) {
		t.Parallel()
		_, err := ParseSortStringStrict("name", nil)
		require.ErrorIs(t, err, ErrSortFieldNotAllowed)
	})
}

func TestParseSortOption(t *testing.T) {
	t.Parallel()
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		d, ok := ParseSortOption("name,-age")
		require.True(t, ok, "expected ok")
		require.Len(t, d, 2)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		_, ok := ParseSortOption("")
		require.False(t, ok, "expected not ok for empty string")
	})

	t.Run("*string", func(t *testing.T) {
		t.Parallel()
		s := "name"
		d, ok := ParseSortOption(&s)
		require.True(t, ok, "expected ok")
		require.Len(t, d, 1)
	})

	t.Run("nil *string", func(t *testing.T) {
		t.Parallel()
		_, ok := ParseSortOption[*string](nil)
		require.False(t, ok, "expected not ok for nil pointer")
	})

	t.Run("bson.D", func(t *testing.T) {
		t.Parallel()
		input := bson.D{{Key: "name", Value: 1}}
		d, ok := ParseSortOption(input)
		require.True(t, ok, "expected ok")
		require.Equal(t, "name", d[0].Key)
	})

	t.Run("empty bson.D", func(t *testing.T) {
		t.Parallel()
		_, ok := ParseSortOption(bson.D{})
		require.False(t, ok, "expected not ok for empty bson.D")
	})
}

func TestParseSortOptionNamedTypes(t *testing.T) {
	t.Parallel()
	type namedString string
	type namedPointer *string
	plain := "name,-age"
	expected := bson.D{{Key: "name", Value: 1}, {Key: "age", Value: -1}}
	parsed, ok := ParseSortOption(namedString(plain))
	require.True(t, ok)
	require.Equal(t, expected, parsed)
	parsed, ok = ParseSortOption(namedPointer(&plain))
	require.True(t, ok)
	require.Equal(t, expected, parsed)
	parsed, ok = ParseSortOption(namedPointer(nil))
	require.False(t, ok)
	require.Nil(t, parsed)
}
