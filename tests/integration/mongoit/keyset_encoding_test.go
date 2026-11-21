// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
	"github.com/altessa-s/go-atlas/store/mongo/keyset"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

func TestKeysetInheritsClientEncoding(t *testing.T) {
	t.Parallel()
	c := client(t)
	db := "mongoit_encoding_" + rand.Text()
	t.Cleanup(func() { _ = c.Database(db).Drop(context.WithoutCancel(t.Context())) })

	type calendarDay struct {
		Year  int
		Month time.Month
		Day   int
	}
	reg := bsoncodec.NewRegistry()
	require.NoError(t, bsoncodec.RegisterDateType[calendarDay](reg))
	var filter bson.Raw
	monitor := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "find" && e.Command.Lookup("find").StringValue() == "items" {
			filter = append(bson.Raw(nil), e.Command.Lookup("filter").Document()...)
		}
	}}
	opts := options.Client().ApplyURI(livebackend.MongoURI()).SetMonitor(monitor).
		SetRegistry(reg).SetBSONOptions(&options.BSONOptions{IntMinSize: true})
	m, err := datamongo.New(db, datamongo.WithClientOptions(opts))
	require.NoError(t, err)
	require.NoError(t, m.Connect(t.Context()))
	t.Cleanup(m.Close)

	coll := m.Database().Collection("items")
	doc := bson.D{{Key: "day", Value: calendarDay{2026, time.November, 21}}, {Key: "count", Value: int64(3)}}
	_, err = coll.InsertOne(t.Context(), doc)
	require.NoError(t, err)
	page, err := keyset.FindPage[bson.Raw](t.Context(), coll, nil, keyset.Query{Filter: doc})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "2026-11-21", filter.Lookup("day").StringValue())
	require.Equal(t, bson.TypeInt32, filter.Lookup("count").Type)

	_, err = keyset.FindPage[bson.Raw](t.Context(), coll, nil, keyset.Query{Filter: doc, BSONOptions: &options.BSONOptions{}})
	require.NoError(t, err)
	require.Equal(t, bson.TypeInt64, filter.Lookup("count").Type)
}
