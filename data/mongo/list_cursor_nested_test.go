// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSortDirectionAfterBsonRoundtrip(t *testing.T) {
	// Sort from ParseSortString has int values
	original := bson.D{{Key: "sort_fields.value", Value: 1}}
	t.Logf("Original sort[0].Value type: %T, value: %v", original[0].Value, original[0].Value)

	// Encode to base64 BSON (same as cursor creation)
	encoded, err := encodeSortToString(original)
	if err != nil {
		t.Fatalf("encodeSortToString: %v", err)
	}

	// Decode back (same as cursor parsing)
	decoded, err := decodeSortFromString(encoded)
	if err != nil {
		t.Fatalf("decodeSortFromString: %v", err)
	}
	t.Logf("Decoded sort[0].Value type: %T, value: %v", decoded[0].Value, decoded[0].Value)

	// Check getSortDirection
	dirOriginal := getSortDirection(original)
	dirDecoded := getSortDirection(decoded)
	t.Logf("Direction original: %d, decoded: %d", dirOriginal, dirDecoded)

	if dirOriginal != dirDecoded {
		t.Errorf("Sort direction changed after BSON round-trip! original=%d, decoded=%d", dirOriginal, dirDecoded)
	}
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
			if got != tt.expected {
				t.Errorf("getSortDirection() = %v, want %v", got, tt.expected)
			}
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
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var itemMap bson.M
	if err = bson.Unmarshal(itemBytes, &itemMap); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	t.Logf("itemMap[sort_fields] type: %T", itemMap["sort_fields"])

	// Test dot notation lookup (bson.D nested doc)
	val := BsonLookup(itemMap, "sort_fields.value")
	if fmt.Sprint(val) != "Агент" {
		t.Fatalf("BsonLookup(sort_fields.value): expected 'Агент', got %v", val)
	}

	// Test simple key (no dot)
	val2 := BsonLookup(itemMap, "_id")
	if fmt.Sprint(val2) != "test-id" {
		t.Fatalf("BsonLookup(_id): expected 'test-id', got %v", val2)
	}

	// Test missing key
	if BsonLookup(itemMap, "missing.key") != nil {
		t.Fatal("BsonLookup(missing.key): expected nil")
	}

	// Test extractCursorDataFromItem uses BsonLookup internally
	sort := bson.D{{Key: "sort_fields.value", Value: 1}, {Key: "cursor_id", Value: 1}}
	cursorId, sortValue, err := extractCursorDataFromItem(item, "cursor_id", sort)
	if err != nil {
		t.Fatalf("extractCursorDataFromItem failed: %v", err)
	}

	if cursorId == "" {
		t.Fatal("cursorId is empty")
	}
	if sortValue == nil {
		t.Fatal("sortValue is nil, expected 'Агент'")
	}
}
