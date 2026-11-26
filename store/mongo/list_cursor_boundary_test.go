// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

func TestCursorNullPredicate(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		direction int
	}{
		{name: "ascending", direction: 1},
		{name: "descending", direction: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id := bson.NewObjectID()
			sort := bson.D{{Key: "rank", Value: tt.direction}}
			cursor, err := NewCursorWithMetadata(id.Hex(), sort, "position", nil, (*int)(nil))
			require.NoError(t, err)

			var match bson.Raw
			coll := mockReadCollection(t, func(_ context.Context, e *event.CommandStartedEvent) {
				if e.CommandName == "aggregate" {
					match = e.Command.Lookup("pipeline").Array().Index(0).Document().Lookup("$match").Document()
					match = append(bson.Raw(nil), match...)
				}
			}, bson.A{})
			_, err = ListCursor[bson.M](t.Context(), coll,
				WithListCursorCursor(cursor), WithListCursorSort(sort), WithListCursorIdField("position"))
			require.NoError(t, err)

			want := bson.M{"rank": nil, "position": bson.M{"$lt": id}}
			if tt.direction > 0 {
				want = bson.M{"$or": bson.A{
					bson.M{"rank": nil, "position": bson.M{"$gt": id}},
					bson.M{"rank": bson.M{"$ne": nil}},
				}}
			}
			var got bson.M
			decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(match)))
			decoder.DefaultDocumentM()
			require.NoError(t, decoder.Decode(&got))
			require.Equal(t, want, got)
		})
	}
}

func TestListCursorUsesRawBoundary(t *testing.T) {
	t.Parallel()

	type privateRank struct {
		ID   bson.ObjectID `bson:"_id"`
		rank int
	}
	type dateModel struct {
		ID   bson.ObjectID `bson:"_id"`
		Rank civil.Date    `bson:"rank"`
	}
	type idOnly struct {
		ID bson.ObjectID `bson:"_id"`
	}

	t.Run("private field", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() { checkListCursorBoundary[privateRank](t, int32(1), nil) })
	})
	t.Run("registered date codec", func(t *testing.T) {
		t.Parallel()
		checkListCursorBoundary[dateModel](t, "2024-01-01", bsoncodec.NewRegistry())
	})
	t.Run("omitted field", func(t *testing.T) {
		t.Parallel()
		checkListCursorBoundary[idOnly](t, int32(1), nil)
	})
}

func checkListCursorBoundary[T any](t *testing.T, rank any, registry *bson.Registry) {
	t.Helper()
	first, second := bson.NewObjectID(), bson.NewObjectID()
	reply := func(items ...bson.D) bson.D {
		return cursorBatchReply("db.items", bson.D{{Key: "items", Value: items}})
	}
	opts := options.Client().SetRegistry(registry)
	opts.Deployment = drivertest.NewMockDeployment(
		reply(bson.D{{Key: "_id", Value: first}, {Key: "rank", Value: rank}},
			bson.D{{Key: "_id", Value: second}, {Key: "rank", Value: rank}}),
		reply(bson.D{{Key: "_id", Value: second}, {Key: "rank", Value: rank}}),
	)
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	coll := client.Database("db").Collection("items")
	listOpts := []ListCursorOption{WithListCursorIdField("_id"), WithListCursorSort("rank"), WithListCursorLimit(1)}
	page, err := ListCursor[T](t.Context(), coll, listOpts...)
	require.NoError(t, err)
	require.NotNil(t, page.NextCursor)
	token, err := ParseCursor(*page.NextCursor)
	require.NoError(t, err)
	require.NotEmpty(t, token.SortValue)
	wire, err := base64.StdEncoding.DecodeString(token.SortValue)
	require.NoError(t, err)
	value, err := bson.Raw(wire).LookupErr("v")
	require.NoError(t, err)
	var decoded any
	require.NoError(t, value.Unmarshal(&decoded))
	require.Equal(t, rank, decoded)
	next, err := ListCursor[T](t.Context(), coll, append(listOpts, WithListCursorCursor(*page.NextCursor))...)
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	require.Nil(t, next.NextCursor)
}

func TestListCursorRejectsMissingBoundary(t *testing.T) {
	t.Parallel()

	sort := bson.D{{Key: "rank", Value: 1}}
	c, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, "_id", nil, nil)
	require.NoError(t, err)
	c.SortValue = ""
	opts := options.Client()
	opts.Deployment = drivertest.NewMockDeployment(cursorBatchReply("db.items"))
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	coll := client.Database("db").Collection("items")
	for _, cursor := range []ListCursorOption{WithListCursorCursor(c), WithListCursorCursor(c.String())} {
		_, err = ListCursor[bson.M](t.Context(), coll,
			WithListCursorIdField("_id"), WithListCursorSort(sort), cursor)
		require.ErrorIs(t, err, ErrInvalidCursor)
	}
}

func cursorBatchReply(namespace string, docs ...bson.D) bson.D {
	if docs == nil {
		docs = []bson.D{}
	}
	return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{
		{Key: "id", Value: int64(0)}, {Key: "ns", Value: namespace}, {Key: "firstBatch", Value: docs},
	}}}
}
