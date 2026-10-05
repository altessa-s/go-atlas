// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// zeroFields declares one field of every kind as zero-when-absent.
func zeroFields() filter.TranslatorOption {
	return filter.WithZeroWhenAbsent(map[string]filter.FieldKind{
		"name":    filter.FieldKindString,
		"age":     filter.FieldKindInt,
		"price":   filter.FieldKindFloat,
		"active":  filter.FieldKindBool,
		"blob":    filter.FieldKindBytes,
		"at":      filter.FieldKindTimestamp,
		"ignored": filter.FieldKindUnspecified,
	})
}

func TestTranslator_ZeroWhenAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, expr, want string
	}{
		// Comparisons: the absent branch follows the stored zero.
		{"equal zero string", `name == ""`, `{"$or":[{"name":""},{"name":{"$exists":false}}]}`},
		{"not equal zero string", `name != ""`, `{"$and":[{"name":{"$ne":""}},{"name":{"$exists":true}}]}`},
		{"equal other string", `name == "x"`, `{"$and":[{"name":"x"},{"name":{"$exists":true}}]}`},
		{"not equal other string", `name != "x"`, `{"$or":[{"name":{"$ne":"x"}},{"name":{"$exists":false}}]}`},
		{"string less than", `name < "a"`, `{"$or":[{"name":{"$lt":"a"}},{"name":{"$exists":false}}]}`},
		{"string greater than", `name > "a"`, `{"$and":[{"name":{"$gt":"a"}},{"name":{"$exists":true}}]}`},
		{"equal zero int", `age == 0`, `{"$or":[{"age":0},{"age":{"$exists":false}}]}`},
		{"int against float", `age < 0.5`, `{"$or":[{"age":{"$lt":0.5}},{"age":{"$exists":false}}]}`},
		{"int less or equal negative", `age <= -1`, `{"$and":[{"age":{"$lte":-1}},{"age":{"$exists":true}}]}`},
		{"int greater or equal zero", `age >= 0`, `{"$or":[{"age":{"$gte":0}},{"age":{"$exists":false}}]}`},
		{"unsigned zero", `age == 0u`, `{"$or":[{"age":0},{"age":{"$exists":false}}]}`},
		{"float zero", `price == 0.0`, `{"$or":[{"price":0},{"price":{"$exists":false}}]}`},
		{"bool false", `active == false`, `{"$or":[{"active":false},{"active":{"$exists":false}}]}`},
		{"bool true", `active == true`, `{"$and":[{"active":true},{"active":{"$exists":true}}]}`},
		{"bool ordering", `active < true`, `{"$or":[{"active":{"$lt":true}},{"active":{"$exists":false}}]}`},
		{"bytes empty", `blob == b""`, `{"$or":[{"blob":""},{"blob":{"$exists":false}}]}`},

		// Incompatible literals behave as a stored zero does in MongoDB.
		{"string field equals number", `name == 0`, `{"$and":[{"name":0},{"name":{"$exists":true}}]}`},
		{"string field not equal number", `name != 0`, `{"$or":[{"name":{"$ne":0}},{"name":{"$exists":false}}]}`},
		{"int field equals string", `age == ""`, `{"$and":[{"age":""},{"age":{"$exists":true}}]}`},
		{"int field ordered by string", `age < "a"`, `{"$and":[{"age":{"$lt":"a"}},{"age":{"$exists":true}}]}`},

		// null never equals a stored value.
		{"equal null", `name == null`, `{"$and":[{"name":null},{"name":{"$exists":true}}]}`},
		{"not equal null", `name != null`, `{"$or":[{"name":{"$ne":null}},{"name":{"$exists":false}}]}`},

		// Membership.
		{"in with zero", `age in [0, 7]`, `{"$or":[{"age":{"$in":[0,7]}},{"age":{"$exists":false}}]}`},
		{"in without zero", `age in [3, 7]`, `{"$and":[{"age":{"$in":[3,7]}},{"age":{"$exists":true}}]}`},
		{"in with mismatched zero", `name in [0]`, `{"$and":[{"name":{"$in":[0]}},{"name":{"$exists":true}}]}`},
		{"in with mixed list", `name in [0, ""]`, `{"$or":[{"name":{"$in":[0,""]}},{"name":{"$exists":false}}]}`},
		{"in with null", `name in [null, "x"]`, `{"$and":[{"name":{"$in":[null,"x"]}},{"name":{"$exists":true}}]}`},

		// String predicates: only an empty needle matches "".
		{"contains empty", `name.contains("")`, `{"$or":[{"name":{"$regex":""}},{"name":{"$exists":false}}]}`},
		{"contains other", `name.contains("a")`, `{"$and":[{"name":{"$regex":"a"}},{"name":{"$exists":true}}]}`},
		{"startsWith empty", `name.startsWith("")`, `{"$or":[{"name":{"$regex":"^"}},{"name":{"$exists":false}}]}`},
		{"endsWith other", `name.endsWith("a")`, `{"$and":[{"name":{"$regex":"a$"}},{"name":{"$exists":true}}]}`},
		{"matches empty", `name.matches("^$")`, `{"$or":[{"name":{"$regex":"^$"}},{"name":{"$exists":false}}]}`},
		{"matches non-empty", `name.matches("^.+$")`, `{"$and":[{"name":{"$regex":"^.+$"}},{"name":{"$exists":true}}]}`},
		{"regex on non-string", `age.contains("")`, `{"$and":[{"age":{"$regex":""}},{"age":{"$exists":true}}]}`},

		// has() always holds; bare and negated identifiers are already exact.
		{"has", `has(row.name)`, `{"row.name":{"$exists":true}}`},
		{"bare", `active`, `{"active":true}`},
		{"negated bare", `!active`, `{"active":{"$ne":true}}`},

		// Negation and nesting compose leaf by leaf.
		{
			"negated comparison", `!(name == "")`,
			`{"$nor":[{"$or":[{"name":""},{"name":{"$exists":false}}]}]}`,
		},
		{
			"or of declared and plain", `active == false || status == "x"`,
			`{"$or":[{"$or":[{"active":false},{"active":{"$exists":false}}]},{"status":"x"}]}`,
		},

		// Undeclared fields, and an Unspecified kind, are untouched.
		{"undeclared field", `status == ""`, `{"status":""}`},
		{"unspecified kind", `ignored == ""`, `{"ignored":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mustTranslator(t, zeroFields()).Translate(testhelpers.MustParseFilter(t, tt.expr))
			require.NoError(t, err)
			require.JSONEq(t, tt.want, bsonToJSON(got))
		})
	}
}

// TestTranslator_ZeroWhenAbsentHas checks has() on a declared field: it
// needs a field selection to parse, so the declaration names the dotted path.
func TestTranslator_ZeroWhenAbsentHas(t *testing.T) {
	t.Parallel()
	trans := mustTranslator(t, filter.WithZeroWhenAbsent(map[string]filter.FieldKind{"row.name": filter.FieldKindString}))

	got, err := trans.Translate(testhelpers.MustParseFilter(t, `has(row.name)`))
	require.NoError(t, err)
	require.JSONEq(t, `{}`, bsonToJSON(got))

	got, err = trans.Translate(testhelpers.MustParseFilter(t, `!has(row.name)`))
	require.NoError(t, err)
	require.JSONEq(t, `{"$nor":[{}]}`, bsonToJSON(got))
}

func TestTranslator_ZeroWhenAbsentSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, expr, wantBranch string
		zeroMatches            bool
	}{
		{"string size zero", `name.size() == 0`, "name", true},
		{"string size not zero", `name.size() != 0`, "name", false},
		{"string size below one", `name.size() < 1`, "name", true},
		{"string size above zero", `name.size() > 0`, "name", false},
		{"string size float", `name.size() <= 0.5`, "name", true},
		{"bytes size zero", `blob.size() == 0`, "blob", false},
		{"bytes size not zero", `blob.size() != 0`, "blob", true},
		{"bytes size below one", `blob.size() < 1`, "blob", false},
		{"int size not zero", `age.size() != 1`, "age", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mustTranslator(t, zeroFields()).Translate(testhelpers.MustParseFilter(t, tt.expr))
			require.NoError(t, err)
			op, other := "$and", map[string]any{"$exists": true}
			if tt.zeroMatches {
				op, other = "$or", map[string]any{"$exists": false}
			}
			branches, ok := got[op].(bson.A)
			require.True(t, ok, "want %s, got %v", op, got)
			require.Len(t, branches, 2)
			require.Equal(t, bson.M{tt.wantBranch: bson.M(other)}, branches[1])
		})
	}
}

func TestTranslator_ZeroWhenAbsentSizeRejectsNonNumbers(t *testing.T) {
	t.Parallel()

	for _, expr := range []string{
		`name.size() < "x"`,
		`name.size() > null`,
		`!(name.size() < "x")`,
		`name.size() == true`,
	} {
		_, err := mustTranslator(t, zeroFields()).Translate(testhelpers.MustParseFilter(t, expr))
		require.ErrorIs(t, err, filter.ErrInvalidExpression, expr)
	}

	// An undeclared field keeps today's behavior.
	_, err := mustTranslator(t).Translate(testhelpers.MustParseFilter(t, `name.size() < "x"`))
	require.NoError(t, err)
}

func TestTranslator_ZeroWhenAbsentTimestamp(t *testing.T) {
	t.Parallel()

	// The zero time is 0001-01-01T00:00:00Z. BSON dates hold milliseconds,
	// so a literal inside that millisecond equals it.
	tests := []struct {
		expr        string
		zeroMatches bool
	}{
		{`at == timestamp("0001-01-01T00:00:00Z")`, true},
		{`at == timestamp("0001-01-01T00:00:00.000999999Z")`, true},
		{`at == timestamp("0001-01-01T00:00:00.001Z")`, false},
		{`at < timestamp("0001-01-01T00:00:00.000999Z")`, false},
		{`at < timestamp("0001-01-01T00:00:00.001Z")`, true},
		{`at in [timestamp("0001-01-01T00:00:00.0005Z")]`, true},
		{`at > timestamp("2024-01-01T00:00:00Z")`, false},
	}

	for _, tt := range tests {
		got, err := mustTranslator(t, zeroFields()).Translate(testhelpers.MustParseFilter(t, tt.expr))
		require.NoError(t, err, tt.expr)
		_, isOr := got["$or"]
		require.Equal(t, tt.zeroMatches, isOr, tt.expr)
	}
}

func TestZeroMatchesComparison(t *testing.T) {
	t.Parallel()

	require.True(t, zeroMatchesComparison(filter.OpNotEqual, filter.FieldKindFloat, math.NaN()))
	require.False(t, zeroMatchesComparison(filter.OpEqual, filter.FieldKindFloat, math.NaN()))
	require.False(t, zeroMatchesComparison(filter.OpLT, filter.FieldKindFloat, math.NaN()))
	require.True(t, zeroMatchesComparison(filter.OpLT, filter.FieldKindInt, uint64(1)))
	require.True(t, zeroMatchesComparison(filter.OpEqual, filter.FieldKindBytes, []byte{}))
	require.False(t, zeroMatchesComparison(filter.OpEqual, filter.FieldKindBytes, []byte(nil)), "a nil slice is encoded as null")
	require.True(t, zeroMatchesComparison(filter.OpNotEqual, filter.FieldKindBytes, []byte(nil)))
	require.False(t, zeroMatchesIn(filter.FieldKindBytes, []any{[]byte(nil)}))
	require.True(t, zeroMatchesIn(filter.FieldKindBytes, []any{[]byte(nil), []byte{}}))
	require.True(t, zeroMatchesComparison(filter.OpGTE, filter.FieldKindTimestamp, time.Time{}))
	require.False(t, zeroMatchesComparison(filter.OpEqual, filter.FieldKindBool, int64(0)))
}
