// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestComputeFilterHash_StableAcrossMapIteration confirms the filter hash is
// deterministic: Go map iteration is randomized, so a filter with a nested
// bson.M of >=2 keys must hash the same across calls — otherwise cursor
// pagination intermittently fails with INVALID_CURSOR.
func TestComputeFilterHash_StableAcrossMapIteration(t *testing.T) {
	t.Parallel()

	filter := bson.M{"$and": bson.A{
		bson.M{"deleted_at": nil, "organization_id": "f3360599-53c9-4833-9bb6-3c75e043caea"},
		bson.M{"cooperation_format": int64(3)},
	}}

	first, err := computeFilterHash(filter)
	require.NoError(t, err)
	for range 1000 {
		got, err := computeFilterHash(filter)
		require.NoError(t, err)
		require.Equal(t, first, got,
			"same filter must produce the same hash regardless of map iteration order")
	}
}

// TestComputeFilterHash_EmptyFilter pins down the empty-filter contract: nil
// and zero-length bson.M share the same fixed hash, distinct from any
// non-empty filter.
func TestComputeFilterHash_EmptyFilter(t *testing.T) {
	t.Parallel()

	empty, err := computeFilterHash(bson.M{})
	require.NoError(t, err)

	nilHash, err := computeFilterHash(nil)
	require.NoError(t, err)
	require.Equal(t, empty, nilHash)

	other, err := computeFilterHash(bson.M{"x": 1})
	require.NoError(t, err)
	require.NotEqual(t, empty, other)
}

// TestComputeFilterHash_DistinctFiltersDistinctHashes guards against silent
// degradation (e.g. always returning the empty-filter hash) by checking that a
// handful of common filter shapes all hash to distinct values.
func TestComputeFilterHash_DistinctFiltersDistinctHashes(t *testing.T) {
	t.Parallel()

	objID := bson.NewObjectID()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	cases := map[string]bson.M{
		"empty":         {},
		"single_string": {"name": "John"},
		"single_int":    {"age": 25},
		"two_fields":    {"age": 25, "name": "John"},
		"nested_and": {"$and": bson.A{
			bson.M{"deleted_at": nil, "organization_id": "f3360599"},
			bson.M{"cooperation_format": int64(3)},
		}},
		"object_id":   {"_id": objID},
		"timestamp":   {"created_at": now},
		"deep_nested": {"a": bson.M{"b": bson.M{"c": bson.M{"d": 1, "e": 2}}}},
	}

	hashes := make(map[string]string, len(cases))
	for name, filter := range cases {
		h, err := computeFilterHash(filter)
		require.NoError(t, err, "case %q", name)
		hashes[name] = h
	}

	seen := make(map[string]string, len(hashes))
	for name, h := range hashes {
		if other, ok := seen[h]; ok {
			t.Fatalf("hash collision between %q and %q: %s", name, other, h)
		}
		seen[h] = name
	}
}

// TestComputeFilterHash_DeepNestingStable extends the regression test to >=3
// levels of nested bson.M, where every level has multiple keys — exactly the
// shape that broke the old fmt.Fprint-based implementation.
func TestComputeFilterHash_DeepNestingStable(t *testing.T) {
	t.Parallel()

	filter := bson.M{
		"$or": bson.A{
			bson.M{"a": 1, "b": bson.M{"c": 2, "d": bson.M{"e": 3, "f": 4}}},
			bson.M{"g": bson.M{"h": 5, "i": 6}, "j": 7},
		},
	}

	first, err := computeFilterHash(filter)
	require.NoError(t, err)
	for range 200 {
		got, err := computeFilterHash(filter)
		require.NoError(t, err)
		require.Equal(t, first, got)
	}
}

// TestComputeFilterHash_UnmarshalableFilterReturnsTypedError verifies that
// computeFilterHash refuses to hash a filter json.Marshal cannot encode (NaN,
// here) and surfaces [ErrCursorFilterUnmarshalable]. This is the fail-loud
// contract that lets callers distinguish a programming error in filter
// construction from a benign hash mismatch.
func TestComputeFilterHash_UnmarshalableFilterReturnsTypedError(t *testing.T) {
	t.Parallel()

	bad := bson.M{"score": math.NaN()}

	_, err := computeFilterHash(bad)
	require.ErrorIs(t, err, ErrCursorFilterUnmarshalable)
}

// TestComputeFilterHash_DistinctBadFiltersBothFail closes the gap where two
// different unmarshalable filters could otherwise share the same error-domain
// hash. With fail-loud semantics, both inputs return [ErrCursorFilterUnmarshalable]
// and no hash is produced — preventing ValidateFilter from accepting a
// mismatched cursor.
func TestComputeFilterHash_DistinctBadFiltersBothFail(t *testing.T) {
	t.Parallel()

	cases := []bson.M{
		{"x": math.NaN()},
		{"y": math.NaN()},
		{"a": math.Inf(1), "b": math.Inf(-1)},
		{"nested": bson.M{"deep": math.NaN()}},
	}

	for _, filter := range cases {
		_, err := computeFilterHash(filter)
		require.ErrorIs(t, err, ErrCursorFilterUnmarshalable)
	}
}

// TestNewCursorWithMetadata_UnmarshalableFilterPropagatesError verifies the
// constructor surfaces the typed error to callers rather than embedding a
// silently-wrong hash in the cursor.
func TestNewCursorWithMetadata_UnmarshalableFilterPropagatesError(t *testing.T) {
	t.Parallel()

	_, err := NewCursorWithMetadata(
		"507f1f77bcf86cd799439011",
		bson.D{{Key: "created_at", Value: -1}},
		"_id",
		bson.M{"score": math.NaN()},
		nil,
	)
	require.ErrorIs(t, err, ErrCursorFilterUnmarshalable)
}

// TestCursorValidateFilter_UnmarshalableFilterPropagatesError verifies the
// validator surfaces the typed error so handlers can tell a malformed filter
// apart from a real mismatch.
func TestCursorValidateFilter_UnmarshalableFilterPropagatesError(t *testing.T) {
	t.Parallel()

	cursor, err := NewCursorWithMetadata(
		"507f1f77bcf86cd799439011",
		bson.D{{Key: "created_at", Value: -1}},
		"_id",
		bson.M{"status": "active"},
		nil,
	)
	require.NoError(t, err)

	err = cursor.ValidateFilter(bson.M{"score": math.NaN()})
	require.ErrorIs(t, err, ErrCursorFilterUnmarshalable)
}
