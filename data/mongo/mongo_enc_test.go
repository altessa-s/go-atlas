// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
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

func TestConvertToNewDocument_OpaqueStructPointer_StoredAsScalar(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	ts := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	entity := &opaqueStructEntity{ID: "f1", Name: "pending", UpdatedAt: &ts}

	doc, err := m.ConvertToNewDocument(context.Background(), entity)
	require.NoError(t, err,
		"non-nil *time.Time must be treated as a scalar leaf, not recursed into")
	require.Equal(t, &ts, doc["updated_at"],
		"expected the *time.Time pointer to be stored as-is for the BSON driver")
	require.Equal(t, "pending", doc["name"])
}

func TestConvertToNewDocument_NilOpaqueStructPointer(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	entity := &opaqueStructEntity{ID: "f2", Name: "active"}

	doc, err := m.ConvertToNewDocument(context.Background(), entity)
	require.NoError(t, err)
	require.Equal(t, "active", doc["name"])
}

func TestConvertToUpdateDocument_OpaqueStructPointer_GoesToSet(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	ts := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	entity := &opaqueStructEntity{ID: "f3", Name: "pending", UpdatedAt: &ts}

	doc, err := m.ConvertToUpdateDocument(context.Background(), entity)
	require.NoError(t, err)

	set, ok := doc["$set"].(bson.M)
	require.True(t, ok, "expected $set document")
	require.Equal(t, &ts, set["updated_at"])
}

func TestConvertToUpdateDocument_NilOpaqueStructPointer_GoesToUnset(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	entity := &opaqueStructEntity{ID: "f4", Name: "active"}

	doc, err := m.ConvertToUpdateDocument(context.Background(), entity)
	require.NoError(t, err)

	unset, ok := doc["$unset"].(bson.M)
	require.True(t, ok, "expected $unset document for the nil pointer field")
	require.Contains(t, unset, "updated_at")
}

func TestConvertToNewDocument_StructPointerWithExportedFields_StillRecursed(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	entity := &nestedParentEntity{ID: "p1", Child: &nestedChild{Value: "v"}}

	doc, err := m.ConvertToNewDocument(context.Background(), entity)
	require.NoError(t, err)

	child, ok := doc["child"].(bson.M)
	require.True(t, ok, "pointer to a regular struct must still be converted recursively")
	require.Equal(t, "v", child["value"])
}

func TestConvertToNewDocument_OpaqueStructsInSliceAndMap(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	entity := &opaqueCollectionsEntity{
		ID:    "c1",
		Times: []time.Time{t1, t2},
		ByKey: map[string]time.Time{"created": t1},
	}

	doc, err := m.ConvertToNewDocument(context.Background(), entity)
	require.NoError(t, err,
		"time.Time elements of slices and maps must not be recursed into")
	require.Equal(t, bson.A{t1, t2}, doc["times"])
	require.Equal(t, bson.M{"created": t1}, doc["by_key"])
}

func TestHasExportedField(t *testing.T) {
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
			require.Equal(t, tt.want, hasExportedField(reflect.TypeOf(tt.target)))
		})
	}
}
