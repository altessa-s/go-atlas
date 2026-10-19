// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/keyset"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
)

type keysetDoc struct {
	ID    any    `bson:"_id"`
	Group string `bson:"group"`
	Rank  int    `bson:"rank"`
}

// keysetColl returns a fresh collection on a throwaway database.
func keysetColl(t *testing.T) *mongo.Collection {
	t.Helper()
	c := client(t)
	name := "mongoit_keyset_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { _ = c.Database(name).Drop(context.Background()) })
	return c.Database(name).Collection("docs")
}

// readAll pages through q and returns every document in page order.
func readAll(t *testing.T, coll *mongo.Collection, codec *keyset.Codec, q datamongo.KeysetQuery) ([]keysetDoc, int) {
	t.Helper()
	var all []keysetDoc
	pages := 0
	for {
		page, err := datamongo.FindPage[keysetDoc](t.Context(), coll, codec, q)
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, int64(len(page.Items)), q.Limit)
		all = append(all, page.Items...)
		if page.Next == "" {
			return all, pages
		}
		q.After = page.Next
	}
}

func TestFindPage_IDTypes(t *testing.T) {
	t.Parallel()
	codec, err := keyset.New(bytes.Repeat([]byte{5}, keyset.MinKeyLength))
	require.NoError(t, err)

	for name, id := range map[string]func(i int) any{
		"object_id": func(int) any { return bson.NewObjectID() },
		"string":    func(i int) any { return fmt.Sprintf("doc-%03d", i) },
		"uuid": func(i int) any {
			return bson.Binary{Subtype: bson.TypeBinaryUUID, Data: bytes.Repeat([]byte{byte(i)}, 16)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			coll := keysetColl(t)
			docs := make([]any, 0, 25)
			for i := range 25 {
				// Ranks repeat, so _id has to break the ties.
				docs = append(docs, keysetDoc{ID: id(i), Group: []string{"a", "b"}[i%2], Rank: i % 4})
			}
			_, err := coll.InsertMany(t.Context(), docs)
			require.NoError(t, err)

			q := datamongo.KeysetQuery{
				Filter:  bson.D{{Key: "group", Value: "a"}},
				Sort:    bson.D{{Key: "rank", Value: -1}},
				Limit:   4,
				Subject: "alice",
			}
			got, pages := readAll(t, coll, codec, q)
			require.Len(t, got, 13, "every matching document exactly once")
			require.Equal(t, 4, pages)
			require.True(t, slices.IsSortedFunc(got, func(a, b keysetDoc) int { return b.Rank - a.Rank }))
			seen := map[string]bool{}
			for _, d := range got {
				require.Equal(t, "a", d.Group)
				key := fmt.Sprint(d.ID)
				require.False(t, seen[key], "duplicate %v", d.ID)
				seen[key] = true
			}
		})
	}
}

func TestFindPage_TokenBindings(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := keyset.New(bytes.Repeat([]byte{6}, keyset.MinKeyLength))
	require.NoError(t, err)
	docs := make([]any, 0, 5)
	for i := range 5 {
		docs = append(docs, keysetDoc{ID: i, Group: "a", Rank: i})
	}
	_, err = coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	q := datamongo.KeysetQuery{Sort: bson.D{{Key: "rank", Value: 1}}, Limit: 2, Subject: "alice"}
	page, err := datamongo.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	other := q
	other.After = page.Next
	other.Filter = bson.D{{Key: "group", Value: "b"}}
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, keyset.ErrFilterChanged)

	other = q
	other.After = page.Next
	other.Sort = bson.D{{Key: "rank", Value: -1}}
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, keyset.ErrSortChanged)

	other = q
	other.After = page.Next
	other.Subject = "mallory"
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, keyset.ErrSubjectMismatch)

	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, nil, datamongo.KeysetQuery{After: page.Next})
	require.ErrorIs(t, err, datamongo.ErrKeysetTokensRequired)
	require.ErrorIs(t, err, keyset.ErrCodecRequired)
}

func TestFindPage_MissingSortValue(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength))
	require.NoError(t, err)
	_, err = coll.InsertMany(t.Context(), []any{
		bson.D{{Key: "_id", Value: 1}, {Key: "rank", Value: 1}},
		bson.D{{Key: "_id", Value: 2}},
		bson.D{{Key: "_id", Value: 3}, {Key: "rank", Value: 3}},
	})
	require.NoError(t, err)

	// Ascending puts the missing value first; it ends the first page.
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec,
		datamongo.KeysetQuery{Sort: bson.D{{Key: "rank", Value: 1}}, Limit: 1})
	require.ErrorIs(t, err, datamongo.ErrKeysetMissingKey)
}

func TestFindPage_Projection(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	_, err := coll.InsertMany(t.Context(), []any{
		keysetDoc{ID: 1, Group: "a", Rank: 1},
		keysetDoc{ID: 2, Group: "a", Rank: 2},
	})
	require.NoError(t, err)
	codec, err := keyset.New(bytes.Repeat([]byte{8}, keyset.MinKeyLength))
	require.NoError(t, err)

	q := datamongo.KeysetQuery{
		Sort:       bson.D{{Key: "rank", Value: 1}},
		Projection: bson.D{{Key: "rank", Value: 1}},
		Limit:      1,
	}
	got, _ := readAll(t, coll, codec, q)
	require.Len(t, got, 2)
	require.Empty(t, got[0].Group, "projected out")

	q.Projection = bson.D{{Key: "rank", Value: 0}}
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.ErrorIs(t, err, datamongo.ErrKeysetMissingKey, "the projection must keep the sort fields")

	q.Projection = bson.D{{Key: "rank", Value: bson.D{{Key: "$literal", Value: 0}}}}
	_, err = datamongo.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.ErrorIs(t, err, datamongo.ErrKeysetProjection, "a computed sort field would corrupt the position")
}
