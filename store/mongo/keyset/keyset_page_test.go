// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas-core/types/optional"
	"github.com/altessa-s/go-atlas/store/mongo/keyset"

	mongokeyset "github.com/altessa-s/go-mongox/keyset"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
)

var _ func(context.Context, *driver.Collection, keyset.TokenCodec, keyset.Query) (keyset.Page[bson.M], error) = keyset.FindPage[bson.M]

// bindingProbe stops FindPage after binding the prepared filter, before the database call.
type bindingProbe struct {
	keyset.TokenCodec
	bindings pagetoken.Bindings
}

var errBound = errors.New("captured bindings")

func (p *bindingProbe) Resolve(_ string, bindings pagetoken.Bindings) ([]byte, error) {
	p.bindings = bindings
	return nil, errBound
}

func boundFilter(tb testing.TB, q keyset.Query) bson.Raw {
	tb.Helper()
	probe := new(bindingProbe)
	q.After = "probe"
	_, err := keyset.FindPage[bson.M](tb.Context(), nil, probe, q)
	require.ErrorIs(tb, err, errBound)
	return bson.Raw(probe.bindings.Filter).Lookup("f").Document()
}

type celsius struct{ deg int32 }
type bounds struct{ Min int64 }
type customDocument struct{}

func (customDocument) MarshalBSON() ([]byte, error) { return bson.Marshal(bson.D{}) }

func TestFindPageFilterAdmission(t *testing.T) {
	t.Parallel()
	reg := bson.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[celsius](), bson.ValueEncoderFunc(
		func(_ bson.EncodeContext, w bson.ValueWriter, v reflect.Value) error {
			c, _ := reflect.TypeAssert[celsius](v)
			return w.WriteInt32(c.deg)
		}))
	for _, v := range []any{bounds{3}, celsius{21}, customDocument{}} {
		_, err := keyset.FindPage[bson.M](t.Context(), nil, nil, keyset.Query{Filter: bson.D{{Key: "value", Value: v}}, Registry: reg})
		require.ErrorIs(t, err, bsonfilter.ErrValueType)
	}

	bsonfilter.AdmitScalar(reg, reflect.TypeFor[celsius]())
	raw, err := bson.Marshal(bson.D{{Key: "min", Value: int64(3)}})
	require.NoError(t, err)
	doc := boundFilter(t, keyset.Query{
		Registry: reg, BSONOptions: &options.BSONOptions{IntMinSize: true},
		Filter: bson.D{
			{Key: "temp", Value: celsius{21}},
			{Key: "ordered", Value: bson.D{{Key: "min", Value: int64(3)}}},
			{Key: "raw", Value: bson.Raw(raw)},
		},
	})
	require.Equal(t, int32(21), doc.Lookup("temp").Int32())
	require.Equal(t, int32(3), doc.Lookup("ordered", "min").Int32())
	require.Equal(t, int64(3), doc.Lookup("raw", "min").Int64())
}

func TestFindPageAtlasRegistry(t *testing.T) {
	t.Parallel()
	doc := boundFilter(t, keyset.Query{Filter: bson.D{{Key: "price", Value: optional.Some(decimal.RequireFromString("10.5"))}}})
	require.Equal(t, "10.5", doc.Lookup("price").Decimal128().String())
}

func TestFindPageAtlasErrors(t *testing.T) {
	t.Parallel()
	_, err := keyset.FindPage[bson.M](t.Context(), nil, nil, keyset.Query{TimestampPrecision: time.Microsecond})
	require.ErrorIs(t, err, keyset.ErrMissingKey)
	require.ErrorIs(t, err, mongokeyset.ErrPrecision)
	for _, operator := range []string{"$where", "$function", "$accumulator"} {
		q := keyset.Query{Filter: bson.D{{Key: "nested", Value: []bson.M{{operator: nil}}}}}
		_, err = keyset.FindPage[bson.M](t.Context(), nil, nil, q)
		require.ErrorIs(t, err, keyset.ErrFilterContainsDangerousOperator)
		require.ErrorIs(t, err, bsonfilter.ErrDangerousOperator)
	}
}

// These tokens were issued and resolved by d59f630 with go-aip v0.1.8.
// map_ab and map_ba came from separate encodings of the same two-key bson.M.
// The fixtures were issued by Atlas d59f630 with go-aip v0.1.8 before the
// migration. The two map fixtures capture both BSON orders of the same input.
func TestLegacyTokenBindings(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/atlas_tokens.json")
	require.NoError(t, err)
	var tokens map[string]string
	require.NoError(t, json.Unmarshal(data, &tokens))
	codec, err := pagetoken.New(bytes.Repeat([]byte{3}, pagetoken.MinKeyLength),
		pagetoken.WithClock(func() time.Time { return time.Unix(1700000000, 0) }))
	require.NoError(t, err)
	ordered := bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}
	q := keyset.Query{Filter: bson.D{{Key: "pair", Value: ordered}}, After: tokens["ordered"], Subject: "alice"}
	expired, err := pagetoken.New(bytes.Repeat([]byte{3}, pagetoken.MinKeyLength),
		pagetoken.WithClock(func() time.Time { return time.Unix(1700000000, 0).Add(pagetoken.DefaultTTL + time.Second) }))
	require.NoError(t, err)
	_, err = keyset.FindPage[bson.M](t.Context(), nil, expired, q)
	require.ErrorIs(t, err, pagetoken.ErrExpiredToken)
	q.Filter = bson.D{{Key: "pair", Value: bson.D{{Key: "a", Value: int64(1)}, {Key: "b", Value: int32(2)}}}}
	_, err = keyset.FindPage[bson.M](t.Context(), nil, codec, q)
	require.ErrorIs(t, err, pagetoken.ErrFilterChanged)
	for _, tc := range []struct {
		name string
		pair any
		want error
	}{
		{"ordered", ordered, nil},
		{"map_ab", bson.M{"b": int32(2), "a": int32(1)}, nil},
		{"map_ba", bson.M{"a": int32(1), "b": int32(2)}, pagetoken.ErrFilterChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			probe := new(bindingProbe)
			q := keyset.Query{Filter: bson.D{{Key: "pair", Value: tc.pair}}, After: tokens[tc.name], Subject: "alice"}
			_, err := keyset.FindPage[bson.M](t.Context(), nil, probe, q)
			require.ErrorIs(t, err, errBound)
			payload, err := codec.Resolve(q.After, probe.bindings)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int32(1), bson.Raw(payload).Lookup("k", "0").Int32())
		})
	}
}
