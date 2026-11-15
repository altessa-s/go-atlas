// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas/store/mongo/keyset"

	aipmongo "github.com/altessa-s/go-aip/backend/mongo"
	filterparser "github.com/altessa-s/go-aip/filter/parser"
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
	name := "mongoit_keyset_" + rand.Text()
	t.Cleanup(func() { _ = c.Database(name).Drop(context.WithoutCancel(t.Context())) })
	return c.Database(name).Collection("docs")
}

// readAll pages through q and returns every document in page order.
func readAll(t *testing.T, coll *mongo.Collection, codec *pagetoken.Codec, q keyset.Query) ([]keysetDoc, int) {
	t.Helper()
	var all []keysetDoc
	pages := 0
	for {
		page, err := keyset.FindPage[keysetDoc](t.Context(), coll, codec, q)
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
	codec, err := pagetoken.New(bytes.Repeat([]byte{5}, pagetoken.MinKeyLength))
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

			q := keyset.Query{
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

// TestFindPage_TranslatedFilter pages with a filter translated afresh for every request, as a list endpoint does: the ordered
// translation yields the same bytes each time, so every token resolves.
func TestFindPage_TranslatedFilter(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := pagetoken.New(bytes.Repeat([]byte{7}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	docs := make([]any, 0, 30)
	for i := range 30 {
		docs = append(docs, keysetDoc{ID: i, Group: []string{"a", "b", "c"}[i%3], Rank: i % 5})
	}
	_, err = coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	parser, err := filterparser.New(filterparser.WithNoCache(true))
	require.NoError(t, err)
	q := keyset.Query{Sort: bson.D{{Key: "rank", Value: -1}}, Limit: 3, Subject: "alice"}
	var got []keysetDoc
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10)
		node, err := parser.Parse(t.Context(), `(group == "a" || group == "b") && !(rank in [0]) && rank >= 1`)
		require.NoError(t, err)
		policy, err := filter.NewPolicy(filter.WithAnyField(true))
		require.NoError(t, err)
		tr, err := aipmongo.NewFilterTranslator(policy)
		require.NoError(t, err)
		q.Filter, err = tr.TranslateD(node)
		require.NoError(t, err)
		page, err := keyset.FindPage[keysetDoc](t.Context(), coll, codec, q)
		require.NoError(t, err)
		got = append(got, page.Items...)
		if page.Next == "" {
			break
		}
		q.After = page.Next
	}
	require.Len(t, got, 16)
	require.True(t, slices.IsSortedFunc(got, func(a, b keysetDoc) int { return b.Rank - a.Rank }))
}

func TestFindPage_TokenBindings(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := pagetoken.New(bytes.Repeat([]byte{6}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	docs := make([]any, 0, 5)
	for i := range 5 {
		docs = append(docs, keysetDoc{ID: i, Group: "a", Rank: i})
	}
	_, err = coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	q := keyset.Query{Sort: bson.D{{Key: "rank", Value: 1}}, Limit: 2, Subject: "alice"}
	page, err := keyset.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	other := q
	other.After = page.Next
	other.Filter = bson.D{{Key: "group", Value: "b"}}
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, pagetoken.ErrFilterChanged)

	other = q
	other.After = page.Next
	other.Sort = bson.D{{Key: "rank", Value: -1}}
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, pagetoken.ErrSortChanged)

	other = q
	other.After = page.Next
	other.Subject = "mallory"
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec, other)
	require.ErrorIs(t, err, pagetoken.ErrSubjectMismatch)

	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, nil, keyset.Query{After: page.Next})
	require.ErrorIs(t, err, keyset.ErrTokensRequired)
	require.ErrorIs(t, err, pagetoken.ErrCodecRequired)
}

func TestFindPage_MissingSortValue(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := pagetoken.New(bytes.Repeat([]byte{7}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	_, err = coll.InsertMany(t.Context(), []any{
		bson.D{{Key: "_id", Value: 1}, {Key: "rank", Value: 1}},
		bson.D{{Key: "_id", Value: 2}},
		bson.D{{Key: "_id", Value: 3}, {Key: "rank", Value: 3}},
	})
	require.NoError(t, err)

	// Ascending puts the missing value first; it ends the first page.
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec,
		keyset.Query{Sort: bson.D{{Key: "rank", Value: 1}}, Limit: 1})
	require.ErrorIs(t, err, keyset.ErrMissingKey)
}

func TestFindPage_Projection(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	_, err := coll.InsertMany(t.Context(), []any{
		keysetDoc{ID: 1, Group: "a", Rank: 1},
		keysetDoc{ID: 2, Group: "a", Rank: 2},
	})
	require.NoError(t, err)
	codec, err := pagetoken.New(bytes.Repeat([]byte{8}, pagetoken.MinKeyLength))
	require.NoError(t, err)

	q := keyset.Query{
		Sort:       bson.D{{Key: "rank", Value: 1}},
		Projection: bson.D{{Key: "rank", Value: 1}},
		Limit:      1,
	}
	got, _ := readAll(t, coll, codec, q)
	require.Len(t, got, 2)
	require.Empty(t, got[0].Group, "projected out")

	q.Projection = bson.D{{Key: "rank", Value: 0}}
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.ErrorIs(t, err, keyset.ErrMissingKey, "the projection must keep the sort fields")

	q.Projection = bson.D{{Key: "rank", Value: bson.D{{Key: "$literal", Value: 0}}}}
	_, err = keyset.FindPage[keysetDoc](t.Context(), coll, codec, q)
	require.ErrorIs(t, err, keyset.ErrProjection, "a computed sort field would corrupt the position")
}

// TestFindPage_DecimalFilter pages with a decimal.Decimal bound: FindPage encodes it with the go-atlas registry as a Decimal128, the
// type the documents store, rather than as an empty document that matches nothing.
func TestFindPage_DecimalFilter(t *testing.T) {
	t.Parallel()
	coll := keysetColl(t)
	codec, err := pagetoken.New(bytes.Repeat([]byte{9}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	docs := make([]any, 0, 10)
	for i := range 10 {
		price, err := bson.ParseDecimal128(strconv.Itoa(i) + ".5")
		require.NoError(t, err)
		docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "rank", Value: i}, {Key: "price", Value: price}})
	}
	_, err = coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	q := keyset.Query{
		Filter: bson.D{{Key: "price", Value: bson.D{{Key: "$gte", Value: decimal.RequireFromString("4.5")}}}},
		Sort:   bson.D{{Key: "rank", Value: 1}},
		Limit:  2,
	}
	got, pages := readAll(t, coll, codec, q)
	require.Len(t, got, 6, "the decimal bound selects ranks 4..9")
	require.Equal(t, 3, pages)
	require.Equal(t, 4, got[0].Rank)
}
