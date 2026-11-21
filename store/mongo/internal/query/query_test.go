// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestValidateFilter_RejectsDangerousOperators is the regression guard:
// every server-side JavaScript operator must be blocked at any
// depth of the filter — including nested inside $expr, $or, $and, and
// $nor arrays.
func TestValidateFilter_RejectsDangerousOperators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		filter bson.M
		op     string
	}{
		{
			name:   "top-level $where",
			filter: bson.M{"$where": "this.x == this.y"},
			op:     "$where",
		},
		{
			name:   "top-level $function",
			filter: bson.M{"$function": bson.M{"body": "function() {}", "lang": "js"}},
			op:     "$function",
		},
		{
			name:   "top-level $accumulator",
			filter: bson.M{"$accumulator": bson.M{}},
			op:     "$accumulator",
		},
		{
			name: "nested in $expr via bson.M",
			filter: bson.M{
				"$expr": bson.M{"$function": bson.M{"body": "function() {}", "lang": "js"}},
			},
			op: "$function",
		},
		{
			name: "nested in $or via []bson.M",
			filter: bson.M{
				"$or": []bson.M{
					{"a": 1},
					{"$where": "true"},
				},
			},
			op: "$where",
		},
		{
			name: "nested in $or via []any",
			filter: bson.M{
				"$or": []any{
					bson.M{"a": 1},
					bson.M{"$where": "true"},
				},
			},
			op: "$where",
		},
		{
			name: "nested in $and via bson.A",
			filter: bson.M{
				"$and": bson.A{
					bson.M{"a": 1},
					bson.M{"$accumulator": bson.M{}},
				},
			},
			op: "$accumulator",
		},
		{
			name: "deeply nested via bson.D",
			filter: bson.M{
				"$expr": bson.D{{Key: "$function", Value: bson.M{}}},
			},
			op: "$function",
		},
		{
			name: "typed array of bson.D",
			filter: bson.M{
				"$or": []bson.D{{{Key: "$where", Value: "true"}}},
			},
			op: "$where",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateFilter(t.Context(), tc.filter, Registry(), nil)
			require.ErrorIs(t, err, ErrDangerousOperator,
				"validateFilter must surface ErrDangerousOperator")
			require.Contains(t, err.Error(), tc.op,
				"error must name the offending operator so logs locate the attacker payload")
		})
	}
}

// TestValidateFilter_AllowsLegitimateOperators confirms the deny-list is
// narrow: common comparison / logical / projection operators must pass
// untouched. Blocking them would break every real query.
func TestValidateFilter_AllowsLegitimateOperators(t *testing.T) {
	t.Parallel()

	cases := []bson.M{
		{"_id": "abc"},
		{"$or": []bson.M{{"a": 1}, {"b": 2}}},
		{"age": bson.M{"$gt": 18, "$lt": 65}},
		{"tags": bson.M{"$in": []string{"x", "y"}}},
		{"$and": bson.A{bson.M{"x": 1}, bson.M{"y": 2}}},
		{"$expr": bson.M{"$eq": []string{"$field1", "$field2"}}}, // $expr without nested $function — safe
		{"name": bson.M{"$regex": "^a", "$options": "i"}},        // regex is a perf hazard, not RCE; allowed
		{"score": bson.M{"$exists": true, "$ne": nil}},
	}

	for i, filter := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			t.Parallel()
			_, err := ValidateFilter(t.Context(), filter, Registry(), nil)
			require.NoError(t, err,
				"filter %v must pass — only RCE-grade operators are blocked", filter)
		})
	}
}

// TestValidateFilter_EmptyAndNil pins the edge cases.
func TestValidateFilter_EmptyAndNil(t *testing.T) {
	t.Parallel()
	_, err := ValidateFilter(t.Context(), nil, Registry(), nil)
	require.NoError(t, err)
	_, err = ValidateFilter(t.Context(), bson.M{}, Registry(), nil)
	require.NoError(t, err)
}
