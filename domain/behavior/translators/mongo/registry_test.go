// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

// money has exported fields, so without a registry that knows it the
// translators fold it as a subdocument; moneyRegistry stores it as a
// Decimal128, as data/mongo/bsoncodec does for decimal.Decimal.
type money struct {
	Units int64
	Scale int32
}

func (m money) decimal128() bson.Decimal128 {
	d, _ := bson.ParseDecimal128FromBigInt(big.NewInt(m.Units), -int(m.Scale))
	return d
}

func moneyRegistry() *bson.Registry {
	reg := bson.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[money](), bson.ValueEncoderFunc(
		func(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
			m, _ := v.Interface().(money)
			return vw.WriteDecimal128(m.decimal128())
		}))
	return reg
}

type order struct {
	ID      string            `bson:"_id"`
	Price   money             `bson:"price"`
	Fee     *money            `bson:"fee"`
	Lines   []money           `bson:"lines"`
	ByCode  map[string]money  `bson:"by_code"`
	Small   money             `bson:"small,minsize"`
	Dynamic any               `bson:"dynamic"`
	Note    string            `bson:"note"`
	Extra   map[string]string `bson:"extra,omitempty"`
}

func newOrder() order {
	return order{
		ID:      "o1",
		Price:   money{Units: 1250, Scale: 2},
		Fee:     &money{Units: 5, Scale: 1},
		Lines:   []money{{Units: 1, Scale: 0}},
		ByCode:  map[string]money{"a": {Units: 3, Scale: 0}},
		Small:   money{Units: 7, Scale: 0},
		Dynamic: money{Units: 9, Scale: 0},
		Note:    "n",
	}
}

// asDecimal unwraps v as the translators may return it: a Decimal128 the
// insert document decoded, or a RawValue / the money value itself left for
// the driver to encode with the same registry.
func asDecimal(t *testing.T, reg *bson.Registry, v any) bson.Decimal128 {
	t.Helper()
	switch x := v.(type) {
	case bson.Decimal128:
		return x
	case bson.RawValue:
		d, ok := x.Decimal128OK()
		require.True(t, ok, "raw %v is not a Decimal128", x.Type)
		return d
	default:
		// The driver encodes a value left as is with the same registry.
		var buf bson.D
		raw := marshalWith(t, reg, bson.D{{Key: "v", Value: v}})
		require.NoError(t, bson.Unmarshal(raw, &buf))
		d, ok := buf[0].Value.(bson.Decimal128)
		require.True(t, ok, "%T does not encode as a Decimal128", v)
		return d
	}
}

func marshalWith(t *testing.T, reg *bson.Registry, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(reg)
	require.NoError(t, enc.Encode(v))
	return buf.Bytes()
}

func requireMoneyEqual(t *testing.T, reg *bson.Registry, want money, got any) {
	t.Helper()
	require.Equal(t, want.decimal128().String(), asDecimal(t, reg, got).String())
}

func TestRegistryInsert(t *testing.T) {
	t.Parallel()

	reg := moneyRegistry()
	o := newOrder()
	got := insert(t, o, mongo.WithRegistry(reg))

	requireMoneyEqual(t, reg, o.Price, got["price"])
	requireMoneyEqual(t, reg, *o.Fee, got["fee"])
	requireMoneyEqual(t, reg, o.Small, got["small"])
	requireMoneyEqual(t, reg, o.Dynamic.(money), got["dynamic"])
	lines, ok := got["lines"].(bson.A)
	require.True(t, ok)
	requireMoneyEqual(t, reg, o.Lines[0], lines[0])
	byCode, ok := got["by_code"].(bson.D)
	require.True(t, ok)
	requireMoneyEqual(t, reg, o.ByCode["a"], byCode[0].Value)
}

func TestRegistryInsertCustomTag(t *testing.T) {
	t.Parallel()

	type doc struct {
		Price money `db:"price"`
	}
	reg := moneyRegistry()
	got := insert(t, doc{Price: money{Units: 42, Scale: 1}}, mongo.WithRegistry(reg), mongo.WithBsonTagName("db"))
	requireMoneyEqual(t, reg, money{Units: 42, Scale: 1}, got["price"])
}

func TestRegistryUpdate(t *testing.T) {
	t.Parallel()

	reg := moneyRegistry()
	o := newOrder()
	got := update(t, o, nil, mongo.WithRegistry(reg))
	set, ok := got["$set"].(bson.M)
	require.True(t, ok)

	requireMoneyEqual(t, reg, o.Price, set["price"])
	requireMoneyEqual(t, reg, *o.Fee, set["fee"])
	requireMoneyEqual(t, reg, o.Small, set["small"])
	requireMoneyEqual(t, reg, o.Dynamic.(money), set["dynamic"])
	lines, ok := set["lines"].(bson.A)
	require.True(t, ok)
	requireMoneyEqual(t, reg, o.Lines[0], lines[0])
	byCode, ok := set["by_code"].(bson.M)
	require.True(t, ok)
	requireMoneyEqual(t, reg, o.ByCode["a"], byCode["a"])
}

func TestRegistryUpdateWithoutRegistryFolds(t *testing.T) {
	t.Parallel()

	got := update(t, newOrder(), nil)
	set, ok := got["$set"].(bson.M)
	require.True(t, ok)
	_, folded := set["price"].(bson.M)
	require.True(t, folded, "without the registry an ordinary struct is a subdocument")
}

func TestRegistryFieldPaths(t *testing.T) {
	t.Parallel()

	got := fieldPaths(t, order{}, behavior.DefaultResponseKinds, mongo.WithRegistry(moneyRegistry()))
	require.Contains(t, got.Selectable, "price")
	require.Contains(t, got.Selectable, "lines")
	for _, p := range got.Selectable {
		require.NotContains(t, p, "price.", "a registry leaf has no selectable subpaths")
		require.NotContains(t, p, "lines.")
	}
}

// pair is a named array type encoded as one string by pairRegistry.
type pair [2]int

func TestRegistryFuncEncoderOnArray(t *testing.T) {
	t.Parallel()

	reg := bson.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[pair](), bson.ValueEncoderFunc(
		func(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
			p, _ := v.Interface().(pair)
			return vw.WriteString(fmt.Sprintf("%d:%d", p[0], p[1]))
		}))
	type doc struct {
		P pair `bson:"p"`
	}
	got := update(t, doc{P: pair{1, 2}}, nil, mongo.WithRegistry(reg))
	set, ok := got["$set"].(bson.M)
	require.True(t, ok)
	raw := marshalWith(t, reg, bson.D{{Key: "p", Value: set["p"]}})
	require.Equal(t, "1:2", bson.Raw(raw).Lookup("p").StringValue(), "the registered encoder writes the array")

	plain := update(t, doc{P: pair{1, 2}}, nil, mongo.WithRegistry(bson.NewRegistry()))
	set, ok = plain["$set"].(bson.M)
	require.True(t, ok)
	_, folded := set["p"].(bson.A)
	require.True(t, folded, "a default registry adds no leaves")
}
