// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (s *Storage) createIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: fieldTimestamp, Value: -1},
				{Key: fieldType, Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: fieldActorID, Value: 1},
				{Key: fieldTimestamp, Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: fieldResourceType, Value: 1},
				{Key: fieldResourceID, Value: 1},
				{Key: fieldTimestamp, Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: fieldContextRequestID, Value: 1},
			},
			Options: mongoOptions.Index().SetSparse(true),
		},
	}

	// Add TTL index if configured.
	if s.opts.ttl > 0 {
		ttlSeconds := int32(s.opts.ttl.Seconds())
		indexes = append(indexes, mongo.IndexModel{
			Keys:    bson.D{{Key: fieldTimestamp, Value: 1}},
			Options: mongoOptions.Index().SetExpireAfterSeconds(ttlSeconds),
		})
	}

	indexCtx, cancel := corecontext.WithDefault(ctx, s.opts.indexTimeout)
	defer cancel()

	_, err := s.collection.Indexes().CreateMany(indexCtx, indexes)
	return err
}
