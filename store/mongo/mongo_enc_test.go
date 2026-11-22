// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// opaqueStructEntity reproduces the bug scenario: a document with a set
// optional timestamp mapped to a non-nil *time.Time.
type opaqueStructEntity struct {
	ID        string     `bson:"_id"`
	Name      string     `bson:"name"`
	UpdatedAt *time.Time `bson:"updated_at"`
}

type nestedChild struct {
	Value string `bson:"value"`
}

type nestedParentEntity struct {
	ID    string       `bson:"_id"`
	Child *nestedChild `bson:"child"`
}

type opaqueCollectionsEntity struct {
	ID    string               `bson:"_id"`
	Times []time.Time          `bson:"times"`
	ByKey map[string]time.Time `bson:"by_key"`
}

// customMoney has an exported field but defines its own BSON serialization via
// bson.ValueMarshaler, so the walker must treat it as a scalar leaf instead of
// recursing into Amount.
type customMoney struct {
	Amount int64
}

func (m customMoney) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue(m.Amount)
	return byte(typ), data, err
}

type marshalerEntity struct {
	ID       string                 `bson:"_id"`
	Price    customMoney            `bson:"price"`
	PricePtr *customMoney           `bson:"price_ptr"`
	Prices   []customMoney          `bson:"prices"`
	ByKey    map[string]customMoney `bson:"by_key"`
}

func TestConvertToNewDocument_OpaqueStructPointer_StoredAsScalar(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	ts := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	entity := &opaqueStructEntity{ID: "f1", Name: "pending", UpdatedAt: &ts}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err,
		"non-nil *time.Time must be treated as a scalar leaf, not recursed into")
	require.Equal(t, bson.NewDateTimeFromTime(ts), doc["updated_at"])
	require.Equal(t, "pending", doc["name"])
}

func TestConvertToNewDocument_NilOpaqueStructPointer(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &opaqueStructEntity{ID: "f2", Name: "active"}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)
	require.Equal(t, "active", doc["name"])
	require.Contains(t, doc, "updated_at")
	require.Nil(t, doc["updated_at"])
}

func TestConvertToUpdateDocument_OpaqueStructPointer_GoesToSet(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	ts := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	entity := &opaqueStructEntity{ID: "f3", Name: "pending", UpdatedAt: &ts}

	doc, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)

	set, ok := doc["$set"].(bson.M)
	require.True(t, ok, "expected $set document")
	raw, err := bson.Marshal(set)
	require.NoError(t, err)
	require.Equal(t, ts.UnixMilli(), bson.Raw(raw).Lookup("updated_at").DateTime())
}

func TestConvertToUpdateDocument_NilOpaqueStructPointer_GoesToUnset(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &opaqueStructEntity{ID: "f4", Name: "active"}

	doc, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)

	unset, ok := doc["$unset"].(bson.M)
	require.True(t, ok, "expected $unset document for the nil pointer field")
	require.Contains(t, unset, "updated_at")
}

func TestConvertToNewDocument_StructPointerWithExportedFields_StillRecursed(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &nestedParentEntity{ID: "p1", Child: &nestedChild{Value: "v"}}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)

	require.Equal(t, bson.D{{Key: "value", Value: "v"}}, doc["child"])
}

func TestConvertToNewDocument_OpaqueStructsInSliceAndMap(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	entity := &opaqueCollectionsEntity{
		ID:    "c1",
		Times: []time.Time{t1, t2},
		ByKey: map[string]time.Time{"created": t1},
	}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err,
		"time.Time elements of slices and maps must not be recursed into")
	require.Equal(t, bson.A{bson.NewDateTimeFromTime(t1), bson.NewDateTimeFromTime(t2)}, doc["times"])
	require.Equal(t, bson.D{{Key: "created", Value: bson.NewDateTimeFromTime(t1)}}, doc["by_key"])
}

func TestConvertToNewDocument_CustomMarshalerStructStoredAsScalar(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	price := customMoney{Amount: 100}
	entity := &marshalerEntity{
		ID:       "m1",
		Price:    price,
		PricePtr: &customMoney{Amount: 200},
		Prices:   []customMoney{{Amount: 1}, {Amount: 2}},
		ByKey:    map[string]customMoney{"eur": {Amount: 9}},
	}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)
	require.Equal(t, int64(100), doc["price"])
	require.Equal(t, int64(200), doc["price_ptr"])
	require.Equal(t, bson.A{int64(1), int64(2)}, doc["prices"])
	require.Equal(t, bson.D{{Key: "eur", Value: int64(9)}}, doc["by_key"])
}

func TestConvertToNewDocument_DriverLayout(t *testing.T) {
	t.Parallel()

	type Embedded struct {
		Name string `bson:"name"`
	}
	type layout struct {
		Embedded
		Fields   Embedded       `bson:",inline"`
		Extra    map[string]int `bson:",inline"`
		Dash     string         `bson:"-,omitempty"`
		Hidden   string         `bson:"-"`
		Number   int64          `bson:"small,minsize"`
		Zero     *int           `bson:"zero,omitempty"`
		Omitted  bool           `bson:"omitted,omitempty"`
		Empty    []int
		Null     map[string]int
		Document bson.D
	}

	// Build the historical bare-tag spelling at runtime so vet can still
	// check every statically declared struct tag in this package.
	bare := reflect.New(reflect.StructOf([]reflect.StructField{{
		Name: "Secret", Type: reflect.TypeFor[string](), Tag: "private",
	}}))
	bare.Elem().Field(0).SetString("s")

	for _, tc := range []struct {
		name   string
		entity any
	}{
		{"value", layout{Embedded: Embedded{"nested"}, Fields: Embedded{"inline"}, Extra: map[string]int{"extra": 1}, Dash: "literal",
			Hidden: "ignored", Number: 10, Zero: new(0), Empty: []int{}, Document: bson.D{{Key: "z", Value: 1}, {Key: "a", Value: 2}}}},
		{"zero pointer", new(layout)},
		{"empty struct", struct{}{}},
		{"bare tag", bare.Interface()},
		{"integer map keys", struct{ Keys map[int]string }{map[int]string{3: "three"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := New("testdb")
			require.NoError(t, err)
			raw, err := bson.Marshal(tc.entity)
			require.NoError(t, err)
			var want bson.M
			require.NoError(t, bson.Unmarshal(raw, &want))
			got, err := m.ConvertToNewDocument(t.Context(), tc.entity)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestConvertToUpdateDocument_ImmutableID(t *testing.T) {
	t.Parallel()
	type ID struct {
		Key *string
	}
	m, err := New("testdb")
	require.NoError(t, err)
	for _, id := range []*ID{nil, {Key: nil}} {
		got, err := m.ConvertToUpdateDocument(t.Context(), struct {
			ID   *ID `bson:"_id"`
			Name *string
		}{ID: id})
		require.NoError(t, err)
		require.Equal(t, bson.M{"$set": bson.M{}, "$unset": bson.M{"name": nil}}, got)
	}
}
