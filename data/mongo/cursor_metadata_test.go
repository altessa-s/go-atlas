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

	first := computeFilterHash(filter)
	for range 1000 {
		require.Equal(t, first, computeFilterHash(filter),
			"same filter must produce the same hash regardless of map iteration order")
	}
}

// TestComputeFilterHash_EmptyFilter pins down the empty-filter contract: nil
// and zero-length bson.M share the same fixed hash, and that hash differs from
// any non-empty filter (including unmarshalable ones, see the NaN test below).
func TestComputeFilterHash_EmptyFilter(t *testing.T) {
	t.Parallel()

	empty := computeFilterHash(bson.M{})
	require.Equal(t, empty, computeFilterHash(nil))
	require.NotEqual(t, empty, computeFilterHash(bson.M{"x": 1}))
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
		hashes[name] = computeFilterHash(filter)
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

	first := computeFilterHash(filter)
	for range 200 {
		require.Equal(t, first, computeFilterHash(filter))
	}
}

// TestComputeFilterHash_UnmarshalableFilterIsNotEmpty ensures a filter that
// json.Marshal cannot encode (NaN float) does NOT collide with the empty-filter
// hash — otherwise ValidateFilter would accept a mismatched cursor.
func TestComputeFilterHash_UnmarshalableFilterIsNotEmpty(t *testing.T) {
	t.Parallel()

	bad := bson.M{"score": math.NaN()}

	require.NotEqual(t, computeFilterHash(bson.M{}), computeFilterHash(bad),
		"unmarshalable filter must not share the empty-filter hash domain")
	require.Equal(t, computeFilterHash(bad), computeFilterHash(bad),
		"unmarshalable filter must still hash deterministically across calls")
}
