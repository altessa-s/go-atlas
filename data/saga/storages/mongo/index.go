// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// instanceIndexes defines MongoDB indexes for the saga instances collection.
var instanceIndexes = []mongo.IndexModel{ //nolint:gochecknoglobals
	// Compound index backing FetchRecoverable: filter by status, then by
	// deadline for the timed-out RUNNING branch. The status prefix also serves
	// the status-only COMPENSATING branch of the recovery scan.
	{Keys: bson.D{
		{Key: collectionFieldStatus, Value: 1},
		{Key: collectionFieldDeadline, Value: 1},
	}},
}

// createIndexes creates the required MongoDB indexes on the instances collection.
// Index creation is idempotent: re-creating an identical index is a no-op.
func (s *Store) createIndexes() error {
	ctx, cancel := corectx.ApplyTimeout(s.ctx, s.indexTimeout)
	defer cancel()

	_, err := s.collection.Indexes().CreateMany(ctx, instanceIndexes, mongoOptions.CreateIndexes())
	if err != nil {
		return coreerrs.WrapOperationWithContext(err, "create MongoDB indexes", "collection '"+s.collectionName+"'")
	}
	return nil
}
