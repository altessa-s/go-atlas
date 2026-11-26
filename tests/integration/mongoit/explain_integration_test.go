// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/explain"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestExplainRun_AndListSingleAggregate(t *testing.T) {
	t.Parallel()

	var aggregateCount atomic.Int64
	var explainCount atomic.Int64
	var queryPlannerCount atomic.Int64
	monitor := &event.CommandMonitor{
		Started: func(_ context.Context, evt *event.CommandStartedEvent) {
			switch evt.CommandName {
			case "aggregate":
				aggregateCount.Add(1)
			case "explain":
				explainCount.Add(1)
				if verbosity, ok := evt.Command.Lookup("verbosity").StringValueOK(); ok && verbosity == string(explain.VerbosityQueryPlanner) {
					queryPlannerCount.Add(1)
				}
			}
		},
	}

	c, err := mongo.Connect(mongoopts.Client().ApplyURI(livebackend.MongoURI()).SetMonitor(monitor))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err = c.Ping(t.Context(), nil); err != nil {
		_ = c.Disconnect(context.WithoutCancel(t.Context()))
		t.Skipf("mongodb not reachable: %v", err)
	}
	t.Cleanup(func() { _ = c.Disconnect(context.WithoutCancel(t.Context())) })

	dbName := "mongoit_explain_" + rand.Text()
	db := c.Database(dbName)
	t.Cleanup(func() { _ = db.Drop(context.WithoutCancel(t.Context())) })

	coll := db.Collection("users")
	_, err = coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: mongoopts.Index().SetName("email_1"),
	})
	require.NoError(t, err)
	_, err = coll.InsertMany(t.Context(), []any{
		bson.D{{Key: "email", Value: "a@example.com"}},
		bson.D{{Key: "email", Value: "b@example.com"}},
	})
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "email", Value: "a@example.com"}}}},
	}

	stats, err := explain.Run(t.Context(), coll, pipeline, nil, logger)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.True(t, stats.IndexUsed)
	require.Equal(t, "email_1", stats.IndexName)

	execStats, err := explain.Run(t.Context(), coll, pipeline, nil, logger, explain.WithVerbosity(explain.VerbosityExecutionStats))
	require.NoError(t, err)
	require.Equal(t, int64(1), execStats.DocsReturned)

	aggregateCount.Store(0)
	explainCount.Store(0)
	queryPlannerCount.Store(0)

	type userDoc struct {
		Email string `bson:"email"`
	}
	res, err := datamongo.List[userDoc](t.Context(), coll,
		datamongo.WithListExplain(),
		datamongo.WithListLogger(logger),
	)
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, int64(1), aggregateCount.Load(), "List with explain must execute only one aggregate command")
	require.Equal(t, int64(1), explainCount.Load(), "List must send one explain command")
	require.Equal(t, int64(1), queryPlannerCount.Load(), "the explain command must use queryPlanner verbosity")
}
