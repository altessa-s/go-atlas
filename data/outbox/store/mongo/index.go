// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// eventsIndexes defines MongoDB indexes for outbox events collection performance.
var eventsIndexes = []mongo.IndexModel{ //nolint:gochecknoglobals
	// Index for fetching unprocessed events (StatusPending or StatusFailed whose
	// backoff has elapsed), and sorting by creation time to process older events first.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},        // Primary filter: by status
		{Key: collectionFieldNextAttemptAt, Value: 1}, // Secondary for StatusFailed with retry backoff
		{Key: collectionFieldCreatedAt, Value: 1},     // Sort order
	}},

	// Index to efficiently find pending events, sorted by creation time.
	// This can be covered by the above but might be used if queries are specific to Pending.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},
		{Key: collectionFieldCreatedAt, Value: 1},
	}},

	// Index for the unlocker task, finding events stuck in StatusInProgress by their LockedOn time.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},   // Filter by StatusInProgress
		{Key: collectionFieldLockedOn, Value: 1}, // Filter by LockedOn time
	}},

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

	// Index creation is generally idempotent in MongoDB.
	// If indexes already exist with the same specification, this operation will be a no-op.
	_, err := s.collection.Indexes().CreateMany(ctx, eventsIndexes, mongoOptions.CreateIndexes()) //nolint:mnd
	if err != nil {
		return coreerrs.WrapOperationWithContext(err, "create MongoDB indexes", "collection '"+s.collectionName+"'")
	}
	return nil
}
