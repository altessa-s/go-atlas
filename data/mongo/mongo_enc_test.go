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
	require.Equal(t, &ts, doc["updated_at"],
		"expected the *time.Time pointer to be stored as-is for the BSON driver")
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
	require.Equal(t, &ts, set["updated_at"])
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

	child, ok := doc["child"].(bson.M)
	require.True(t, ok, "pointer to a regular struct must still be converted recursively")
	require.Equal(t, "v", child["value"])
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
	require.Equal(t, bson.A{t1, t2}, doc["times"])
	require.Equal(t, bson.M{"created": t1}, doc["by_key"])
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
	require.NoError(t, err,
		"a struct implementing bson.ValueMarshaler must be stored as-is, not walked")

	require.Equal(t, price, doc["price"],
		"value field must keep the custom-marshaler struct, not a recursed bson.M")
	require.Equal(t, &customMoney{Amount: 200}, doc["price_ptr"],
		"pointer field must keep the custom-marshaler struct pointer")
	require.Equal(t, bson.A{customMoney{Amount: 1}, customMoney{Amount: 2}}, doc["prices"],
		"slice elements must not be recursed into")
	require.Equal(t, bson.M{"eur": customMoney{Amount: 9}}, doc["by_key"],
		"map values must not be recursed into")
}

func TestHasExportedField(t *testing.T) {
	t.Parallel()

	type exported struct{ A string }
	type unexported struct{ a string } //nolint:unused // exercised via reflection
	type empty struct{}

	tests := []struct {
		name   string
		target any
		want   bool
	}{
		{"struct with exported field", exported{}, true},
		{"struct with only unexported fields", unexported{}, false},
		{"empty struct", empty{}, false},
		{"time.Time", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, hasExportedField(reflect.TypeOf(tt.target)))
		})
	}
}
