// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/keyset"
)

func TestBuildFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kv   []any
		want bson.M
	}{
		{"no args", nil, bson.M{}},
		{"single pair", []any{"status", "active"}, bson.M{"status": "active"}},
		{"multiple pairs", []any{"status", "active", "age", 21}, bson.M{"status": "active", "age": 21}},
		{"nil value kept", []any{"deleted_at", nil}, bson.M{"deleted_at": nil}},
		{"duplicate key last wins", []any{"a", 1, "a", 2}, bson.M{"a": 2}},
		// Odd trailing element has no value and is silently dropped.
		{"odd trailing key dropped", []any{"a", 1, "b"}, bson.M{"a": 1}},
		{"single key without value dropped", []any{"a"}, bson.M{}},
		// Non-string keys fall back to fmt.Sprint instead of panicking.
		{"non-string key stringified", []any{42, "v"}, bson.M{"42": "v"}},
		{"nested operator value", []any{"age", bson.M{"$gte": 18}}, bson.M{"age": bson.M{"$gte": 18}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BuildFilter(tt.kv...)
			require.NotNil(t, got, "BuildFilter must always return a usable map")
			require.Equal(t, tt.want, got)
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

func TestCapListOffset(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	require.Equal(t, int64(0), capListOffset(0, logger), "zero offset must pass through unchanged")
	require.Equal(t, int64(MaxListOffset), capListOffset(MaxListOffset, logger),
		"offset at exactly the cap is permitted")
	require.Equal(t, int64(MaxListOffset-1), capListOffset(MaxListOffset-1, logger),
		"offset just below the cap passes through")
	require.Equal(t, int64(MaxListOffset), capListOffset(MaxListOffset+1, logger),
		"offset just above the cap is clamped")
	require.Equal(t, int64(MaxListOffset), capListOffset(1_000_000, logger),
		"pathological deep-skip request is clamped to MaxListOffset")
}

// TestGenerateDeduplicationKey_DistinguishesTypeSiblings is the regression
// guard for the BSON-aware fix: values that share a fmt.Sprint
// representation but differ by Go type (and therefore by BSON type) must
// produce DIFFERENT singleflight keys. Before the fix, the fmt.Fprint
// path silently coalesced these into the same key, so the second caller
// inherited the first caller's result for a semantically different query.

func TestGenerateDeduplicationKey_DistinguishesTypeSiblings(t *testing.T) {
	cases := []struct {
		name string
		a    bson.M
		b    bson.M
	}{
		{
			name: "int32 vs int64",
			a:    bson.M{"v": int32(1)},
			b:    bson.M{"v": int64(1)},
		},
		{
			name: "string vs bytes",
			a:    bson.M{"v": "1"},
			b:    bson.M{"v": []byte("1")},
		},
		{
			name: "int vs string",
			a:    bson.M{"v": 1},
			b:    bson.M{"v": "1"},
		},
		{
			name: "float64 vs int64",
			a:    bson.M{"v": float64(1)},
			b:    bson.M{"v": int64(1)},
		},
		{
			name: "bool true vs string true",
			a:    bson.M{"v": true},
			b:    bson.M{"v": "true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ka := generateDeduplicationKey("db:coll", tc.a)
			kb := generateDeduplicationKey("db:coll", tc.b)
			require.NotEqual(t, ka, kb,
				"distinct BSON types must yield distinct singleflight keys — otherwise queries get incorrectly coalesced (a=%v b=%v)", tc.a, tc.b)
		})
	}
}

// TestGenerateDeduplicationKey_StableAcrossMapIteration confirms the
// dependency on sorted keys is preserved — Go map iteration is randomized,
// so the function MUST produce the same key for the same logical filter
// across calls.
func TestGenerateDeduplicationKey_StableAcrossMapIteration(t *testing.T) {
	filter := bson.M{"a": 1, "b": "two", "c": int64(3), "d": []byte("four")}

	first := generateDeduplicationKey("p", filter)
	for range 100 {
		require.Equal(t, first, generateDeduplicationKey("p", filter),
			"same filter must produce the same key regardless of map iteration order")
	}
}

// TestGenerateDeduplicationKey_DistinctFiltersDiverge sanity-checks the
// basic correctness path: changing any key or value MUST produce a
// different key.
func TestGenerateDeduplicationKey_DistinctFiltersDiverge(t *testing.T) {
	base := generateDeduplicationKey("p", bson.M{"a": 1, "b": 2})

	require.NotEqual(t, base, generateDeduplicationKey("p", bson.M{"a": 1, "b": 3}),
		"different values must diverge")
	require.NotEqual(t, base, generateDeduplicationKey("p", bson.M{"a": 1, "b": 2, "c": 3}),
		"extra key must diverge")
	require.NotEqual(t, base, generateDeduplicationKey("q", bson.M{"a": 1, "b": 2}),
		"different prefix must diverge")
}

// TestGenerateDeduplicationKey_EmptyFilter pins the early-return path.
func TestGenerateDeduplicationKey_EmptyFilter(t *testing.T) {
	require.Equal(t, "p", generateDeduplicationKey("p", nil))
	require.Equal(t, "p", generateDeduplicationKey("p", bson.M{}))
}

func TestQueryHelpersRejectDangerousFilters(t *testing.T) {
	t.Parallel()
	filter := bson.M{"$or": []bson.D{{{Key: "$where", Value: "true"}}}}

	_, err := List[struct{}](t.Context(), nil, WithListFilter(filter))
	require.ErrorIs(t, err, ErrFilterContainsDangerousOperator)

	_, err = ListCursor[struct{}](t.Context(), nil, WithListCursorFilter(filter))
	require.ErrorIs(t, err, ErrFilterContainsDangerousOperator)

	_, err = keyset.FindPage[struct{}](t.Context(), (*mongo.Collection)(nil), nil, keyset.Query{
		Filter: bson.D{{Key: "$where", Value: "true"}},
	})
	require.ErrorIs(t, err, ErrFilterContainsDangerousOperator)

	var sequenceErr error
	for _, sequenceErr = range GetEntitiesSeq[struct{}, struct{}](t.Context(), nil, nil, filter) {
		break
	}
	require.ErrorIs(t, sequenceErr, ErrFilterContainsDangerousOperator)
}
