// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	mongostore "github.com/altessa-s/go-atlas/reliability/saga/storages/mongo"
)

var (
	// ErrClientRequired is returned when the MongoDB saga store is built
	// without a database.
	ErrClientRequired = errors.New("saga: MongoDB database is required for storage type mongo")
	// ErrConfigRequired is returned when the storage configuration has no
	// mongo block.
	ErrConfigRequired = errors.New("saga: mongo configuration is required for storage type mongo")
)

// Builder returns the storage function of the mongo type, ready for
// sagafactory.Builder.UseStorage. The store keeps instances in the configured
// collection of db; ctx bounds the index creation.
func Builder(db *mongo.Database) sagafactory.StorageFunc {
	return func(ctx context.Context, cfg *sagaconfig.StorageConfig) (saga.Storage, error) {
		if db == nil {
			return nil, ErrClientRequired
		}
		if cfg.Mongo == nil {
			return nil, ErrConfigRequired
		}
		// Index creation runs on ctx through WithContext.
		return mongostore.New(db, //nolint:contextcheck // index creation runs on ctx via WithContext
			mongostore.WithCollectionName(cfg.Mongo.Collection),
			mongostore.WithContext(ctx),
		)
	}
}
