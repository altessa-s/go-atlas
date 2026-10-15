// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	cursormemory "github.com/altessa-s/go-atlas/data/mongo/cursorstorages/memory"
)

func TestListCursor_ScalarContinuation(t *testing.T) {
	t.Parallel()
	for _, stateful := range []bool{false, true} {
		name := "stateless"
		if stateful {
			name = "stateful"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, direction := range []int{1, -1} {
				name := "ascending"
				if direction == -1 {
					name = "descending"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					coll := keysetColl(t)
					docs := make([]any, 0, 9)
					for i := range 9 {
						docs = append(docs, bson.M{"_id": bson.NewObjectID(), "created_at": bson.DateTime(i / 3), "tenant": "A"})
					}
					_, err := coll.InsertMany(t.Context(), docs)
					require.NoError(t, err)
					opts := []datamongo.ListCursorOption{
						datamongo.WithListCursorIdField("_id"),
						datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: direction}}),
						datamongo.WithListCursorFilter(bson.M{"tenant": "A"}),
						datamongo.WithListCursorLimit(2),
						datamongo.WithListCursorSubject("tenant-A"),
					}
					if stateful {
						opts = append(opts, datamongo.WithListCursorStorage(cursormemory.New(time.Minute)))
					}
					seen := make(map[bson.ObjectID]bool)
					var token *string
					var previous bson.DateTime
					for page := 0; ; page++ {
						require.Less(t, page, 5, "pagination must terminate")
						pageOpts := append(opts[:len(opts):len(opts)], datamongo.WithListCursorCursor(token))
						result, err := datamongo.ListCursor[bson.M](t.Context(), coll, pageOpts...)
						require.NoError(t, err)
						for _, item := range result.Items {
							id := item["_id"].(bson.ObjectID)
							require.False(t, seen[id], "duplicate item")
							value := item["created_at"].(bson.DateTime)
							if len(seen) > 0 {
								require.GreaterOrEqual(t, int64(value-previous)*int64(direction), int64(0))
							}
							seen[id] = true
							previous = value
						}
						token = result.NextCursor
						if token == nil {
							break
						}
					}
					require.Len(t, seen, len(docs), "all matching items must appear exactly once")
				})
			}
		})
	}
}
