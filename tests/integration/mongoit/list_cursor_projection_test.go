// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

const projectionDocs = 25

type listedUser struct {
	ID        bson.ObjectID `bson:"_id"`
	Name      string        `bson:"name"`
	Email     string        `bson:"email,omitempty"`
	CreatedAt time.Time     `bson:"created_at"`
}

// projectionColl seeds projectionDocs users on a throwaway database. Five
// users share each created_at, so the pages resume on the _id tie-breaker.
func projectionColl(t *testing.T) *mongo.Collection {
	t.Helper()
	c := client(t)
	name := "mongoit_projection_" + rand.Text()
	t.Cleanup(func() { _ = c.Database(name).Drop(context.WithoutCancel(t.Context())) })
	coll := c.Database(name).Collection("users")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := make([]any, 0, projectionDocs)
	for i := range projectionDocs {
		docs = append(docs, bson.M{
			"_id":        bson.NewObjectID(), // ListCursor resumes on ObjectID cursors
			"name":       fmt.Sprintf("user %d", i),
			"email":      fmt.Sprintf("u%d@example.com", i),
			"created_at": base.Add(time.Duration(i%5) * time.Hour),
		})
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)
	return coll
}

func TestListCursor_PagesUnderProjection(t *testing.T) {
	t.Parallel()
	coll := projectionColl(t)

	proj := bson.M{"name": 1, "_id": 1, "created_at": 1}
	var (
		seen   []string
		cursor *string
	)
	for page := 0; ; page++ {
		require.Less(t, page, projectionDocs, "pagination does not terminate")
		opts := []datamongo.ListCursorOption{
			datamongo.WithListCursorIdField("_id"),
			datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: 1}}),
			datamongo.WithListCursorProjection(proj),
			datamongo.WithListCursorLimit(4),
		}
		if cursor != nil {
			opts = append(opts, datamongo.WithListCursorCursor(cursor))
		}
		res, err := datamongo.ListCursor[listedUser](t.Context(), coll, opts...)
		require.NoError(t, err)
		for _, u := range res.Items {
			require.Empty(t, u.Email, "email was not projected")
			require.NotEmpty(t, u.Name)
			seen = append(seen, u.ID.Hex())
		}
		if res.NextCursor == nil {
			break
		}
		cursor = res.NextCursor
	}
	slices.Sort(seen)
	require.Len(t, seen, projectionDocs)
	require.Len(t, slices.Compact(seen), projectionDocs, "no document is skipped or repeated")
}

func TestListCursor_RejectsProjectionDroppingSortKey(t *testing.T) {
	t.Parallel()
	coll := projectionColl(t)

	_, err := datamongo.ListCursor[listedUser](t.Context(), coll,
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: 1}}),
		datamongo.WithListCursorProjection(bson.M{"name": 1, "_id": 1}),
	)
	require.ErrorIs(t, err, datamongo.ErrProjectionDropsCursorField)
}

func TestListCursor_EmptyProjection(t *testing.T) {
	t.Parallel()
	coll := projectionColl(t)

	res, err := datamongo.ListCursor[listedUser](t.Context(), coll,
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorProjection(bson.M{}),
		datamongo.WithListCursorLimit(2),
	)
	require.NoError(t, err, "an empty projection must not reach the server as $project: {}")
	require.Len(t, res.Items, 2)
	require.NotEmpty(t, res.Items[0].Email)
}
