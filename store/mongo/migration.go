// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type schemaSetup struct {
	registry          *migrations.Registry
	rollbackOnFailure bool
	logger            *slog.Logger
}

func (s *schemaSetup) migrate(ctx context.Context, database *mongo.Database) error {
	// Waiting for a peer's migration and running our own each have a separate budget.
	ctx, cancel := corecontext.WithMaxTimeout(ctx, migrations.LockWaitTimeout+migrations.MigrationTimeout)
	defer cancel()
	opts := []migrations.Option{migrations.WithLogger(s.logger), migrations.WithRegistry(s.registry)}
	if s.rollbackOnFailure {
		opts = append(opts, migrations.WithRollbackOnFailure())
	}
	return migrations.Run(ctx, database, opts...)
}

// createKeyVaultCollection creates the key vault collection for encryption keys.
func createKeyVaultCollection(ctx context.Context, database *mongo.Database, collection string) error {
	// Create the key vault collection.
	_, err := database.
		RunCommand(ctx, bson.D{{Key: "create", Value: collection}}).
		Raw()

	if err != nil {
		serr, ok := errors.AsType[mongo.ServerError](err)
		if !ok || !serr.HasErrorCode(MongoErrorCodeCollectionExists) {
			return err
		}
	}

	keyVaultCollection := database.Collection(collection)

	_, err = keyVaultCollection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "keyAltNames", Value: 1}},
		Options: mongoOptions.Index().
			SetUnique(true).
			SetPartialFilterExpression(bson.D{
				{Key: "keyAltNames", Value: bson.D{{Key: "$exists", Value: true}}},
			}),
	})

	return err
}
