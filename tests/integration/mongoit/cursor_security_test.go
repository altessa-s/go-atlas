// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	cursormemory "github.com/altessa-s/go-atlas/store/mongo/storages/memory"
	cursorredis "github.com/altessa-s/go-atlas/store/mongo/storages/redis"
)

// A stored cursor is bound to the collection and the sort it was issued for.
// The record goes through Redis, so the binding survives the storage encoding.
func TestListCursor_StatefulBinding(t *testing.T) {
	t.Parallel()

	rdb := livebackend.ConnectRedis(t)
	storage := cursorredis.New(rdb, cursorredis.WithKeyPrefix(livebackend.RedisPrefix(t, rdb, "mongoit_cursor")))

	coll := keysetColl(t)
	docs := make([]any, 0, 6)
	for i := range 6 {
		docs = append(docs, bson.M{"_id": bson.NewObjectID(), "created_at": bson.DateTime(i), "tenant": "A"})
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	sort := bson.D{{Key: "created_at", Value: 1}}
	base := []datamongo.ListCursorOption{
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorFilter(bson.M{"tenant": "A"}),
		datamongo.WithListCursorLimit(2),
		datamongo.WithListCursorSubject("tenant-A"),
		datamongo.WithListCursorStorage(storage),
	}
	withSort := func(opts ...datamongo.ListCursorOption) []datamongo.ListCursorOption {
		return append(append(base[:len(base):len(base)], datamongo.WithListCursorSort(sort)), opts...)
	}

	first, err := datamongo.ListCursor[bson.M](t.Context(), coll, withSort()...)
	require.NoError(t, err)
	require.NotNil(t, first.NextCursor)
	token := datamongo.WithListCursorCursor(first.NextCursor)

	second, err := datamongo.ListCursor[bson.M](t.Context(), coll, withSort(token)...)
	require.NoError(t, err, "a continuation repeating the first page's parameters")
	require.Len(t, second.Items, 2)
	require.NotEqual(t, first.Items[0]["_id"], second.Items[0]["_id"])

	_, err = datamongo.ListCursor[bson.M](t.Context(), coll.Database().Collection("other"), withSort(token)...)
	require.ErrorIs(t, err, datamongo.ErrCursorCollectionMismatch)

	_, err = datamongo.ListCursor[bson.M](t.Context(), coll, append(base[:len(base):len(base)], token)...)
	require.ErrorIs(t, err, datamongo.ErrCursorChecksumMismatch, "an omitted sort is the default sort")

	_, err = datamongo.ListCursor[bson.M](t.Context(), coll,
		append(base[:len(base):len(base)], datamongo.WithListCursorSort("-created_at"), token)...)
	require.ErrorIs(t, err, datamongo.ErrCursorChecksumMismatch)
}

func TestListCursor_ScalarContinuation(t *testing.T) {
	t.Parallel()
	// The subject needs a mode that enforces it: unsigned stateless tokens page without one.
	modes := []struct {
		name string
		opts func(t *testing.T) []datamongo.ListCursorOption
	}{
		{name: "stateless", opts: func(*testing.T) []datamongo.ListCursorOption { return nil }},
		{name: "signed", opts: func(t *testing.T) []datamongo.ListCursorOption {
			return []datamongo.ListCursorOption{
				datamongo.WithListCursorSigner(cursorSigner(t)),
				datamongo.WithListCursorSubject("tenant-A"),
			}
		}},
		{name: "stateful", opts: func(*testing.T) []datamongo.ListCursorOption {
			return []datamongo.ListCursorOption{
				datamongo.WithListCursorStorage(cursormemory.New(time.Minute)),
				datamongo.WithListCursorSubject("tenant-A"),
			}
		}},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
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
					}
					opts = append(opts, mode.opts(t)...)
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
