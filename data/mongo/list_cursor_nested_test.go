// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSortDirectionAfterBsonRoundtrip(t *testing.T) {
	// Sort from ParseSortString has int values
	original := bson.D{{Key: "sort_fields.value", Value: 1}}
	t.Logf("Original sort[0].Value type: %T, value: %v", original[0].Value, original[0].Value)

	// Encode to base64 BSON (same as cursor creation)
	encoded, err := encodeSortToString(original)
	require.NoError(t, err, "encodeSortToString")

	// Decode back (same as cursor parsing)
	decoded, err := decodeSortFromString(encoded)
	require.NoError(t, err, "decodeSortFromString")
	t.Logf("Decoded sort[0].Value type: %T, value: %v", decoded[0].Value, decoded[0].Value)

	// Check getSortDirection
	dirOriginal := getSortDirection(original)
	dirDecoded := getSortDirection(decoded)
	t.Logf("Direction original: %d, decoded: %d", dirOriginal, dirDecoded)

	require.Equal(t, dirOriginal, dirDecoded, "Sort direction changed after BSON round-trip")
}

func TestGetSortDirection_Int32(t *testing.T) {
	tests := []struct {
		name     string
		sort     bson.D
		expected int
	}{
		{"int ASC", bson.D{{Key: "created_at", Value: 1}}, sortDirectionAscending},
		{"int DESC", bson.D{{Key: "created_at", Value: -1}}, sortDirectionDescending},
		{"int32 ASC", bson.D{{Key: "created_at", Value: int32(1)}}, sortDirectionAscending},
		{"int32 DESC", bson.D{{Key: "created_at", Value: int32(-1)}}, sortDirectionDescending},
		{"int64 ASC", bson.D{{Key: "created_at", Value: int64(1)}}, sortDirectionAscending},
		{"int64 DESC", bson.D{{Key: "created_at", Value: int64(-1)}}, sortDirectionDescending},
		{"empty sort", bson.D{}, sortDirectionDescending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSortDirection(tt.sort)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestBsonLookup(t *testing.T) {
	// Simulate dictionaryItem with SortFields
	type testItem struct {
		Id         string            `bson:"_id"`
		CursorId   bson.ObjectID     `bson:"cursor_id"`
		SortFields map[string]string `bson:"sort_fields"`
	}

	item := testItem{
		Id:       "test-id",
		CursorId: bson.NewObjectID(),
		SortFields: map[string]string{
			"value":    "Агент",
			"category": "Специалисты",
		},
	}

	// Marshal to BSON then unmarshal to bson.M (same as extractCursorDataFromItem)
	itemBytes, err := bson.Marshal(item)
	require.NoError(t, err, "Marshal failed")

	var itemMap bson.M
	err = bson.Unmarshal(itemBytes, &itemMap)
	require.NoError(t, err, "Unmarshal failed")

	t.Logf("itemMap[sort_fields] type: %T", itemMap["sort_fields"])

	// Test dot notation lookup (bson.D nested doc)
	val := BsonLookup(itemMap, "sort_fields.value")
	require.Equal(t, "Агент", fmt.Sprint(val), "BsonLookup(sort_fields.value)")

	// Test simple key (no dot)
	val2 := BsonLookup(itemMap, "_id")
	require.Equal(t, "test-id", fmt.Sprint(val2), "BsonLookup(_id)")

	// Test missing key
	require.Nil(t, BsonLookup(itemMap, "missing.key"), "BsonLookup(missing.key): expected nil")

	// Test extractCursorDataFromItem uses BsonLookup internally
	sort := bson.D{{Key: "sort_fields.value", Value: 1}, {Key: "cursor_id", Value: 1}}
	cursorId, sortValue, err := extractCursorDataFromItem(item, "cursor_id", sort)
	require.NoError(t, err, "extractCursorDataFromItem failed")
	require.NotEmpty(t, cursorId, "cursorId is empty")
	require.NotNil(t, sortValue, "sortValue is nil, expected 'Агент'")
}
