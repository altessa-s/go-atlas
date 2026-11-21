// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

func TestPreparedReadsInheritClientEncoding(t *testing.T) {
	t.Parallel()
	c := client(t)
	db := "mongoit_reads_" + rand.Text()
	t.Cleanup(func() { _ = c.Database(db).Drop(context.WithoutCancel(t.Context())) })
	type day struct {
		Year  int
		Month time.Month
		Day   int
	}
	reg := bsoncodec.NewRegistry()
	require.NoError(t, bsoncodec.RegisterDateType[day](reg))
	settings := &options.BSONOptions{IntMinSize: true}
	filter := bson.M{"count": int64(3), "day": day{2026, time.November, 21}}
	want, err := bsonfilter.Prepare(t.Context(), filter, reg, settings)
	require.NoError(t, err)
	var commands []bson.Raw
	monitor := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		if (e.CommandName == "find" || e.CommandName == "aggregate") && e.Command.Lookup(e.CommandName).StringValue() == "items" {
			commands = append(commands, bson.Raw(bytes.Clone(e.Command)))
		}
	}}
	opts := options.Client().ApplyURI(livebackend.MongoURI()).SetMonitor(monitor).SetRegistry(reg).SetBSONOptions(settings)
	m, err := datamongo.New(db, datamongo.WithClientOptions(opts))
	require.NoError(t, err)
	require.NoError(t, m.Connect(t.Context()))
	t.Cleanup(m.Close)
	coll := m.Database().Collection("items")
	_, err = coll.InsertMany(t.Context(), []any{filter, filter})
	require.NoError(t, err)
	type item struct{ Count int }
	one, err := datamongo.GetEntity[item, item](t.Context(), m, coll, filter)
	require.NoError(t, err)
	require.Equal(t, 3, one.Count)
	many, err := datamongo.GetEntities[item, item](t.Context(), m, coll, filter)
	require.NoError(t, err)
	require.Len(t, many, 2)
	var count int
	for _, err := range datamongo.GetEntitiesSeq[item, item](t.Context(), m, coll, filter) {
		require.NoError(t, err)
		count++
	}
	require.Equal(t, 2, count)
	page, err := datamongo.List[bson.M](t.Context(), coll, datamongo.WithListFilter(filter), datamongo.WithListTotal(true))
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total)
	count = 0
	for _, err := range datamongo.ListCursorSeq[bson.M](t.Context(), coll,
		datamongo.WithListCursorFilter(filter),
		datamongo.WithListCursorTotal(true),
		datamongo.WithListCursorLimit(1),
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorSort("_id"),
		datamongo.WithListCursorSigner(cursorSigner(t)),
	) {
		require.NoError(t, err)
		count++
		// Later pages must keep the filter captured at the start of iteration.
		filter["count"] = int64(99)
	}
	require.Equal(t, 2, count)
	require.Len(t, commands, 6)
	for _, cmd := range commands[:3] {
		require.Equal(t, want, cmd.Lookup("filter").Document())
	}
	for i, cmd := range commands[3:] {
		stages, err := cmd.Lookup("pipeline").Array().Values()
		require.NoError(t, err)
		match := stages[0].Document().Lookup("$match").Document()
		if i == 2 {
			match = match.Lookup("$and").Array().Index(0).Document()
		}
		require.Equal(t, want, match)
		if i == 0 {
			continue
		}
		var countFilter bson.Raw
		for _, stage := range stages {
			if v, ok := stage.Document().Lookup("$unionWith", "pipeline").ArrayOK(); ok {
				countFilter = v.Index(0).Document().Lookup("$match").Document()
			}
		}
		require.Equal(t, want, countFilter)
	}
}
