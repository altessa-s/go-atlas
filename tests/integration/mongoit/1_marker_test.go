// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"

	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// markerCollection holds the document migration 1 writes and its rollback
// deletes.
const markerCollection = "markers"

func init() {
	migrate.MustRegister(func(ctx context.Context, db *mongo.Database) error {
		_, err := db.Collection(markerCollection).InsertOne(ctx, bson.M{"_id": "marker"})
		return err
	}, func(ctx context.Context, db *mongo.Database) error {
		_, err := db.Collection(markerCollection).DeleteOne(ctx, bson.M{"_id": "marker"})
		return err
	})
}
