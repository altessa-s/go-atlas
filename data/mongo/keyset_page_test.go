// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/keyset"
)

func TestKeysetKeys(t *testing.T) {
	t.Parallel()

	keys, err := keysetKeys(bson.D{{Key: "rank", Value: -1}, {Key: "name", Value: int32(1)}})
	require.NoError(t, err)
	require.Equal(t, []keysetKey{{"rank", true}, {"name", false}, {"_id", false}}, keys, "_id tie-breaker appended")

	keys, err = keysetKeys(bson.D{{Key: "_id", Value: int64(-1)}, {Key: "ignored", Value: 1}})
	require.NoError(t, err)
	require.Equal(t, []keysetKey{{"_id", true}}, keys, "nothing breaks a tie after _id")

	keys, err = keysetKeys(nil)
	require.NoError(t, err)
	require.Equal(t, []keysetKey{{"_id", false}}, keys)

	for name, sort := range map[string]bson.D{
		"direction_2":   {{Key: "a", Value: 2}},
		"text_score":    {{Key: "a", Value: bson.D{{Key: "$meta", Value: "textScore"}}}},
		"string_dir":    {{Key: "a", Value: "asc"}},
		"empty_field":   {{Key: "", Value: 1}},
		"operator":      {{Key: "$natural", Value: 1}},
		"repeated_name": {{Key: "a", Value: 1}, {Key: "a", Value: -1}},
	} {
		_, err := keysetKeys(sort)
		require.ErrorIs(t, err, ErrKeysetSort, name)
	}
}

func TestKeysetPositionRoundTrip(t *testing.T) {
	t.Parallel()

	keys := []keysetKey{{"meta.rank", true}, {"_id", false}}
	doc, err := bson.Marshal(bson.D{{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: 7}}}})
	require.NoError(t, err)

	payload, err := encodeKeysetPosition(doc, keys)
	require.NoError(t, err)
	values, err := decodeKeysetPosition(payload, len(keys))
	require.NoError(t, err)
	require.Equal(t, int32(7), values[0].Int32())
	require.Equal(t, "x1", values[1].StringValue())

	_, err = decodeKeysetPosition(payload, 3)
	require.ErrorIs(t, err, keyset.ErrInvalidToken, "a key count mismatch is a bad token")
	_, err = decodeKeysetPosition([]byte("garbage"), 2)
	require.ErrorIs(t, err, keyset.ErrInvalidToken)

	for name, d := range map[string]bson.D{
		"missing": {{Key: "_id", Value: "x1"}},
		"null":    {{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: nil}}}},
		"array":   {{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: bson.A{1}}}}},
	} {
		raw, err := bson.Marshal(d)
		require.NoError(t, err)
		_, err = encodeKeysetPosition(raw, keys)
		require.ErrorIs(t, err, ErrKeysetMissingKey, name)
	}
}

func TestWithKeysetSeek(t *testing.T) {
	t.Parallel()

	v := func(x any) bson.RawValue {
		typ, data, err := bson.MarshalValue(x)
		require.NoError(t, err)
		return bson.RawValue{Type: typ, Value: data}
	}
	keys := []keysetKey{{"rank", true}, {"_id", false}}
	got := withKeysetSeek(bson.D{{Key: "group", Value: "a"}}, keys, []bson.RawValue{v(3), v("id9")})

	want := bson.D{{Key: "$and", Value: bson.A{
		bson.D{{Key: "group", Value: "a"}},
		bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "rank", Value: bson.D{{Key: "$lt", Value: v(3)}}}},
			bson.D{
				{Key: "rank", Value: bson.D{{Key: "$eq", Value: v(3)}}},
				{Key: "_id", Value: bson.D{{Key: "$gt", Value: v("id9")}}},
			},
		}}},
	}}}
	gotJSON, err := bson.MarshalExtJSON(got, true, false)
	require.NoError(t, err)
	wantJSON, err := bson.MarshalExtJSON(want, true, false)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))

	noFilter := withKeysetSeek(nil, keys[1:], []bson.RawValue{v("id9")})
	require.Equal(t, "$or", noFilter[0].Key)
}

func TestKeysetBindings(t *testing.T) {
	t.Parallel()

	keys, err := keysetKeys(bson.D{{Key: "rank", Value: -1}})
	require.NoError(t, err)
	a, err := keysetBindings(KeysetQuery{Filter: bson.D{{Key: "g", Value: "a"}}, Subject: "s"}, keys)
	require.NoError(t, err)
	b, err := keysetBindings(KeysetQuery{Filter: bson.D{{Key: "g", Value: "b"}}, Subject: "s"}, keys)
	require.NoError(t, err)
	require.NotEqual(t, a.Filter, b.Filter)
	require.Equal(t, a.Sort, b.Sort)

	none, err := keysetBindings(KeysetQuery{}, keys)
	require.NoError(t, err)
	empty, err := keysetBindings(KeysetQuery{Filter: bson.D{}}, keys)
	require.NoError(t, err)
	require.Equal(t, none.Filter, empty.Filter, "nil and empty filters match the same documents")
}

func TestFindPageRejectsTokenWithoutCodec(t *testing.T) {
	t.Parallel()

	codec, err := keyset.New(bytes.Repeat([]byte{1}, keyset.MinKeyLength))
	require.NoError(t, err)
	token, err := codec.Issue([]byte("x"), keyset.Bindings{})
	require.NoError(t, err)
	_, err = FindPage[bson.M](t.Context(), nil, nil, KeysetQuery{After: token})
	require.ErrorIs(t, err, ErrKeysetTokensRequired)
}

func TestKeysetBindingsKeepTypes(t *testing.T) {
	t.Parallel()

	keys, err := keysetKeys(nil)
	require.NoError(t, err)
	a, err := keysetBindings(KeysetQuery{Filter: bson.D{{Key: "rank", Value: bson.D{{Key: "$eq", Value: int32(1)}}}}}, keys)
	require.NoError(t, err)
	b, err := keysetBindings(KeysetQuery{Filter: bson.D{{Key: "rank", Value: bson.D{{Key: "$eq",
		Value: bson.D{{Key: "$numberInt", Value: "1"}}}}}}}, keys)
	require.NoError(t, err)
	require.NotEqual(t, a.Filter, b.Filter, "an int32 and a $numberInt document are different filters")
}

func TestCheckKeysetProjection(t *testing.T) {
	t.Parallel()

	keys := []keysetKey{{"meta.rank", false}, {"_id", false}}
	for _, p := range []bson.D{
		nil,
		{{Key: "meta.rank", Value: 1}, {Key: "name", Value: bson.D{{Key: "$literal", Value: 0}}}},
		{{Key: "meta", Value: true}},
		{{Key: "secret", Value: 0}},
		{{Key: "meta.rank", Value: int64(1)}, {Key: "_id", Value: 0.0}},
	} {
		require.NoError(t, checkKeysetProjection(p, keys), "%v", p)
	}
	for _, p := range []bson.D{
		{{Key: "meta.rank", Value: bson.D{{Key: "$literal", Value: 0}}}},
		{{Key: "meta", Value: "$other"}},
		{{Key: "meta.rank.x", Value: bson.D{{Key: "$slice", Value: 1}}}},
		{{Key: "meta.rank.$", Value: 1}},
		{{Key: "_id", Value: 2}},
	} {
		require.ErrorIs(t, checkKeysetProjection(p, keys), ErrKeysetProjection, "%v", p)
	}
}
