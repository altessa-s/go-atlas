// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-aip/backend/mongo"
	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-aip/filter/filtertest"
	"github.com/altessa-s/go-aip/pagination/pagetoken"
)

func TestKeys(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(bson.D{{Key: "rank", Value: -1}, {Key: "name", Value: int32(1)}}, "")
	require.NoError(t, err)
	require.Equal(t, []sortKey{{"rank", true}, {"name", false}, {"_id", false}}, keys, "_id tie-breaker appended")

	keys, err = sortKeys(bson.D{{Key: "_id", Value: int64(-1)}, {Key: "ignored", Value: 1}}, "")
	require.NoError(t, err)
	require.Equal(t, []sortKey{{"_id", true}}, keys, "nothing breaks a tie after _id")

	keys, err = sortKeys(nil, "")
	require.NoError(t, err)
	require.Equal(t, []sortKey{{"_id", false}}, keys)

	for name, sort := range map[string]bson.D{
		"direction_2":   {{Key: "a", Value: 2}},
		"text_score":    {{Key: "a", Value: bson.D{{Key: "$meta", Value: "textScore"}}}},
		"string_dir":    {{Key: "a", Value: "asc"}},
		"empty_field":   {{Key: "", Value: 1}},
		"operator":      {{Key: "$natural", Value: 1}},
		"repeated_name": {{Key: "a", Value: 1}, {Key: "a", Value: -1}},
	} {
		_, err := sortKeys(sort, "")
		require.ErrorIs(t, err, ErrSort, name)
	}
}

func TestPositionRoundTrip(t *testing.T) {
	t.Parallel()

	keys := []sortKey{{"meta.rank", true}, {"_id", false}}
	doc, err := bson.Marshal(bson.D{{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: 7}}}})
	require.NoError(t, err)

	payload, err := encodePosition(doc, keys)
	require.NoError(t, err)
	values, err := decodePosition(payload, len(keys))
	require.NoError(t, err)
	require.Equal(t, int32(7), values[0].Int32())
	require.Equal(t, "x1", values[1].StringValue())

	_, err = decodePosition(payload, 3)
	require.ErrorIs(t, err, pagetoken.ErrInvalidToken, "a key count mismatch is a bad token")
	_, err = decodePosition([]byte("garbage"), 2)
	require.ErrorIs(t, err, pagetoken.ErrInvalidToken)

	for name, d := range map[string]bson.D{
		"missing": {{Key: "_id", Value: "x1"}},
		"null":    {{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: nil}}}},
		"array":   {{Key: "_id", Value: "x1"}, {Key: "meta", Value: bson.D{{Key: "rank", Value: bson.A{1}}}}},
	} {
		raw, err := bson.Marshal(d)
		require.NoError(t, err)
		_, err = encodePosition(raw, keys)
		require.ErrorIs(t, err, ErrMissingKey, name)
	}
}

func TestWithSeek(t *testing.T) {
	t.Parallel()

	v := func(x any) bson.RawValue {
		typ, data, err := bson.MarshalValue(x)
		require.NoError(t, err)
		return bson.RawValue{Type: typ, Value: data}
	}
	keys := []sortKey{{"rank", true}, {"_id", false}}
	got := withSeek(mustFilter(t, bson.D{{Key: "group", Value: "a"}}), keys, []bson.RawValue{v(3), v("id9")})

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

	noFilter := withSeek(mustFilter(t, nil), keys[1:], []bson.RawValue{v("id9")})
	require.Equal(t, "$or", noFilter[0].Key)
	require.Len(t, noFilter, 1, "an empty filter adds no $and")
}

func mustFilter(tb testing.TB, filter bson.D) bson.Raw {
	tb.Helper()
	raw, err := marshalFilter(filter, nil, nil)
	require.NoError(tb, err)
	return raw
}

func mustBindings(tb testing.TB, filter bson.D, keys []sortKey, subject string) pagetoken.Bindings {
	tb.Helper()
	b, err := newBindings(mustFilter(tb, filter), keys, subject)
	require.NoError(tb, err)
	return b
}

func TestBindings(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(bson.D{{Key: "rank", Value: -1}}, "")
	require.NoError(t, err)
	a := mustBindings(t, bson.D{{Key: "g", Value: "a"}}, keys, "s")
	b := mustBindings(t, bson.D{{Key: "g", Value: "b"}}, keys, "s")
	require.NotEqual(t, a.Filter, b.Filter)
	require.Equal(t, a.Sort, b.Sort)
	require.Equal(t, "s", a.Subject)

	none := mustBindings(t, nil, keys, "")
	empty := mustBindings(t, bson.D{}, keys, "")
	require.Equal(t, none.Filter, empty.Filter, "nil and empty filters match the same documents")
}

func TestFilterBindsTheExecutedBytes(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(nil, "")
	require.NoError(t, err)
	// An embedded-document literal is compared by MongoDB in its key order: the executed filter and the binding keep it.
	filter := bson.D{{Key: "meta", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 2}}}}
	raw := mustFilter(t, filter)
	want, err := bson.Marshal(filter)
	require.NoError(t, err)
	require.Equal(t, bson.Raw(want), raw, "the filter is executed as written")
	elems, err := raw.Lookup("meta").Document().Elements()
	require.NoError(t, err)
	require.Equal(t, "b", elems[0].Key())
	require.Equal(t, "a", elems[1].Key())

	bound := mustBindings(t, filter, keys, "")
	fingerprint, err := bson.Marshal(bson.D{{Key: "f", Value: filter}})
	require.NoError(t, err)
	require.Equal(t, string(fingerprint), bound.Filter, "the binding is the executed bytes")

	reordered := mustBindings(t, bson.D{{Key: "meta", Value: bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 1}}}}, keys, "")
	require.NotEqual(t, bound.Filter, reordered.Filter, "a reordered literal is another filter")
}

func TestFindPageRejectsTokenWithoutCodec(t *testing.T) {
	t.Parallel()

	codec, err := pagetoken.New(bytes.Repeat([]byte{1}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	token, err := codec.Issue([]byte("x"), pagetoken.Bindings{})
	require.NoError(t, err)
	_, err = FindPage[bson.M](t.Context(), nil, nil, Query{After: token})
	require.ErrorIs(t, err, ErrTokensRequired)
	require.ErrorIs(t, err, pagetoken.ErrCodecRequired)
	require.Equal(t, pagetoken.FailureMalformed, pagetoken.Classify(err))
}

func TestBindingsKeepTypes(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(nil, "")
	require.NoError(t, err)
	a := mustBindings(t, bson.D{{Key: "rank", Value: bson.D{{Key: "$eq", Value: int32(1)}}}}, keys, "")
	b := mustBindings(t, bson.D{{Key: "rank", Value: bson.D{{Key: "$eq",
		Value: bson.D{{Key: "$numberInt", Value: "1"}}}}}}, keys, "")
	require.NotEqual(t, a.Filter, b.Filter, "an int32 and a $numberInt document are different filters")
}

func TestCheckProjection(t *testing.T) {
	t.Parallel()

	keys := []sortKey{{"meta.rank", false}, {"_id", false}}
	for _, p := range []bson.D{
		nil,
		{{Key: "meta.rank", Value: 1}, {Key: "name", Value: bson.D{{Key: "$literal", Value: 0}}}},
		{{Key: "meta", Value: true}},
		{{Key: "secret", Value: 0}},
		{{Key: "meta.rank", Value: int64(1)}, {Key: "_id", Value: 0.0}},
	} {
		require.NoError(t, checkProjection(p, keys), "%v", p)
	}
	for _, p := range []bson.D{
		{{Key: "meta.rank", Value: bson.D{{Key: "$literal", Value: 0}}}},
		{{Key: "meta", Value: "$other"}},
		{{Key: "meta.rank.x", Value: bson.D{{Key: "$slice", Value: 1}}}},
		{{Key: "meta.rank.$", Value: 1}},
		{{Key: "_id", Value: 2}},
	} {
		require.ErrorIs(t, checkProjection(p, keys), ErrProjection, "%v", p)
	}
}

func TestBindingsOfATranslatedFilterAreStable(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(bson.D{{Key: "rank", Value: -1}}, "")
	require.NoError(t, err)
	schema, err := mongo.NewSchema(mongo.WithSchemaZeroWhenAbsent(map[string]filter.FieldKind{"note": filter.FieldKindString}))
	require.NoError(t, err)
	tr, err := mongo.NewFilterTranslatorWithSchema(filtertest.MustPolicy(t, filter.WithAnyField(true)), schema)
	require.NoError(t, err)
	node := filtertest.MustParseFilter(t, `(rank > 3 || tags.size() <= 2) && note.startsWith("a") && !(kind in ["x", "y"])`)

	first, err := tr.TranslateD(node)
	require.NoError(t, err)
	want := mustBindings(t, first, keys, "s")
	for range 20 {
		again, err := tr.TranslateD(node)
		require.NoError(t, err)
		require.Equal(t, want, mustBindings(t, again, keys, "s"), "the same filter binds the same token")
	}
}

func TestFilterEncodesDecimalsWithTheConnectRegistry(t *testing.T) {
	t.Parallel()

	keys, err := sortKeys(nil, "")
	require.NoError(t, err)
	price := func(v string) bson.D {
		return bson.D{{Key: "price", Value: bson.D{{Key: "$gte", Value: decimal.RequireFromString(v)}}}}
	}
	raw := mustFilter(t, price("10.5"))
	got := raw.Lookup("price", "$gte")
	require.Equal(t, bson.TypeDecimal128, got.Type, "a decimal filter value is a Decimal128, not an empty document")
	require.Equal(t, "10.5", got.Decimal128().String())

	require.NotEqual(t, mustBindings(t, price("10.5"), keys, "").Filter, mustBindings(t, price("99"), keys, "").Filter,
		"different decimal bounds are different filters")
}

// celsius is a filter value only a custom registry can encode.
type celsius struct{ deg int32 }

func TestFilterUsesTheQueryRegistryAndOptions(t *testing.T) {
	t.Parallel()

	reg := bson.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[celsius](), bson.ValueEncoderFunc(
		func(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
			return vw.WriteInt32(v.Interface().(celsius).deg) //nolint:forcetypeassert // registered for celsius only
		}))
	raw, err := marshalFilter(bson.D{{Key: "temp", Value: celsius{deg: 21}}}, reg, nil)
	require.NoError(t, err)
	require.Equal(t, int32(21), raw.Lookup("temp").Int32(), "the custom codec of the query registry encodes the filter")

	type bounds struct {
		Min int64 `bson:"min"`
	}
	raw, err = marshalFilter(bson.D{{Key: "range", Value: bounds{Min: 3}}}, nil, &options.BSONOptions{IntMinSize: true})
	require.NoError(t, err)
	require.Equal(t, bson.TypeInt32, raw.Lookup("range", "min").Type, "the collection's encoder settings apply")
}

func TestConfiguredIdentityStopsSort(t *testing.T) {
	t.Parallel()
	keys, err := sortKeys(bson.D{{Key: "rank", Value: -1}, {Key: "identity.uuid", Value: -1}}, "identity.uuid")
	require.NoError(t, err)
	require.Equal(t, []sortKey{{field: "rank", desc: true}, {field: "identity.uuid", desc: true}}, keys)
	keys, err = sortKeys(nil, "identity.uuid")
	require.NoError(t, err)
	require.Equal(t, []sortKey{{field: "identity.uuid"}}, keys)
	_, err = sortKeys(nil, "$bad")
	require.ErrorIs(t, err, ErrSort)
}

func TestCustomIdentityCursorWithoutMongoID(t *testing.T) {
	t.Parallel()
	keys, err := sortKeys(nil, "identity.sequence")
	require.NoError(t, err)
	document, err := bson.Marshal(bson.D{{Key: "identity", Value: bson.D{{Key: "sequence", Value: int64(42)}}}})
	require.NoError(t, err)
	payload, err := encodePosition(document, keys)
	require.NoError(t, err)
	values, err := decodePosition(payload, len(keys))
	require.NoError(t, err)
	require.Equal(t, int64(42), values[0].Int64())
	seek := withSeek(mustFilter(t, nil), keys, values)
	require.Equal(t, bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "identity.sequence", Value: bson.D{{Key: "$gt", Value: values[0]}}}}}}}, seek)
	require.ErrorIs(t, checkProjection(bson.D{{Key: "identity", Value: bson.D{{Key: "$literal", Value: 0}}}}, keys), ErrProjection)
}

func TestCursorTimestampPrecision(t *testing.T) {
	t.Parallel()
	keys, err := sortKeys(bson.D{{Key: "at", Value: 1}}, "sequence")
	require.NoError(t, err)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	document, err := bson.Marshal(bson.D{{Key: "at", Value: at}, {Key: "sequence", Value: int64(42)}})
	require.NoError(t, err)
	payload, err := encodePositionPrecision(document, keys, time.Second)
	require.NoError(t, err)
	values, err := decodePosition(payload, len(keys))
	require.NoError(t, err)
	require.Equal(t, at.UnixMilli(), values[0].DateTime())
	require.Equal(t, int64(42), values[1].Int64())
	document, err = bson.Marshal(bson.D{{Key: "at", Value: at.Add(time.Millisecond)}, {Key: "sequence", Value: int64(42)}})
	require.NoError(t, err)
	_, err = encodePositionPrecision(document, keys, time.Second)
	require.ErrorIs(t, err, ErrMissingKey)
}
