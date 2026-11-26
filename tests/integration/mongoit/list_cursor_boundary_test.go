// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	atlas "github.com/altessa-s/go-atlas/store/mongo"
)

type cursorBoundaryID struct {
	ID bson.ObjectID `bson:"_id"`
}

func boundaryCollection(t *testing.T) *mongo.Collection {
	t.Helper()
	db := client(t).Database("mongoit_boundary_" + rand.Text())
	t.Cleanup(func() { require.NoError(t, db.Drop(context.WithoutCancel(t.Context()))) })
	return db.Collection("items")
}

func TestListCursorPagesMissingAndNullSortKeys(t *testing.T) {
	t.Parallel()
	coll := boundaryCollection(t)
	var docs []any
	var ascending []bson.ObjectID
	for i := range 35 {
		id := bson.NewObjectID()
		ascending = append(ascending, id)
		doc := bson.M{"_id": id, "position": id}
		switch {
		case i < 15: // missing
		case i < 18:
			doc["rank"] = nil
		default:
			doc["rank"] = int32(i / 3)
		}
		docs = append(docs, doc)
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	for _, direction := range []int{1, -1} {
		name := "ascending"
		want := slices.Clone(ascending)
		if direction < 0 {
			name = "descending"
			slices.Reverse(want)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ids := listBoundaryIDs(t, coll, direction, 2)
			require.Equal(t, want, ids, "each document must appear once, in sort order")
		})
	}
}

func TestListCursorPagesIDOnlyResultsInSortOrder(t *testing.T) {
	t.Parallel()
	coll := boundaryCollection(t)
	lower, higher := bson.NewObjectID(), bson.NewObjectID()
	_, err := coll.InsertMany(t.Context(), []bson.M{
		{"_id": higher, "position": higher, "rank": 1},
		{"_id": lower, "position": lower, "rank": 2},
	})
	require.NoError(t, err)

	require.Equal(t, []bson.ObjectID{higher, lower}, listBoundaryIDs(t, coll, 1, 1))
}

func listBoundaryIDs(t *testing.T, coll *mongo.Collection, direction int, limit int64) []bson.ObjectID {
	t.Helper()
	var ids []bson.ObjectID
	var token *string
	for page := range 40 {
		result, err := atlas.ListCursor[cursorBoundaryID](t.Context(), coll,
			atlas.WithListCursorIdField("position"),
			atlas.WithListCursorSort(bson.D{{Key: "rank", Value: direction}}),
			atlas.WithListCursorLimit(limit),
			atlas.WithListCursorCursor(token),
			atlas.WithListCursorTotal(false))
		require.NoError(t, err, "page %d", page)
		for _, item := range result.Items {
			ids = append(ids, item.ID)
		}
		token = result.NextCursor
		if token == nil {
			return ids
		}
	}
	t.Fatal("pagination did not terminate")
	return nil
}
