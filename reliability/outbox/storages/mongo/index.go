// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoindexes "github.com/altessa-s/go-atlas/store/mongo/indexes"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// fetchIndexKeys is the index FetchUnprocessedEvents reads each ready status
// through, in fetch order. It is passed as a hint: the planner may otherwise
// pick another status-led index that cannot supply the sort, and sort the whole
// backlog in memory to return one batch.
var fetchIndexKeys = bson.D{ //nolint:gochecknoglobals
	{Key: collectionFieldStatus, Value: 1},
	{Key: collectionFieldCreatedAt, Value: 1},
	{Key: collectionFieldSeq, Value: 1},
}

// eventsIndexes defines MongoDB indexes for outbox events collection performance.
var eventsIndexes = []mongo.IndexModel{ //nolint:gochecknoglobals
	// Index for fetching unprocessed events: one equality on status (pending, or
	// failed), then creation order, so each status is read in fetch order.
	{Keys: fetchIndexKeys},

	// Index for the unlocker task, finding events stuck in StatusInProgress by their LockedOn time.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},   // Filter by StatusInProgress
		{Key: collectionFieldLockedOn, Value: 1}, // Filter by LockedOn time
	}},

	// Index for the expiry sweep, finding pending and failed events by their ExpiresAt deadline.
	// Partial: events without a TTL omit expires_at and stay out of it, so it
	// costs nothing on their writes. The sweep's expires_at range implies the
	// $exists filter, so the planner still picks it.
	{
		Keys: bson.D{
			{Key: collectionFieldStatus, Value: 1},
			{Key: collectionFieldExpiresAt, Value: 1},
		},
		Options: mongoOptions.Index().SetPartialFilterExpression(bson.D{
			{Key: collectionFieldExpiresAt, Value: bson.D{{Key: opExists, Value: true}}},
		}),
	},

	// Index for the cleaner task, finding successfully sent events (StatusSent) by their PublishedAt time.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},      // Filter by StatusSent
		{Key: collectionFieldPublishedAt, Value: 1}, // Filter by PublishedAt time
	}},
}

// createIndexes creates required MongoDB indexes on the event collection.
func (s *Store) createIndexes() error {
	ctx, cancel := corectx.ApplyTimeout(s.ctx, s.indexTimeout)
	defer cancel()

	// Idempotent: indexes that already exist with the same specification are a no-op.
	if err := mongoindexes.Ensure(ctx, s.collection, eventsIndexes); err != nil {
		return coreerrs.WrapOperationWithContext(err, "create MongoDB indexes", "collection '"+s.collectionName+"'")
	}
	return nil
}
