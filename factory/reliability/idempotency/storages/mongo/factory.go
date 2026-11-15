// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	idempotencyfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	mongostorage "github.com/altessa-s/go-atlas/reliability/idempotency/storages/mongo"
)

var (
	// ErrClientRequired is returned when the MongoDB idempotency storage is
	// built without a database.
	ErrClientRequired = errors.New("idempotency: mongo database is required for storage type mongo")
	// ErrConfigRequired is returned when the storage configuration has no
	// mongo block.
	ErrConfigRequired = errors.New("idempotency: mongo configuration is required for storage type mongo")
)

// Builder returns the storage function of the mongo type, ready for
// idempotencyfactory.KeeperBuilder.UseStorage. The storage keeps keys in the
// configured collection of db, expiring them after the configured TTL.
func Builder(db *mongo.Database) idempotencyfactory.StorageFunc {
	return func(cfg *idempotencyconfig.Config) (storages.Storage, error) {
		if db == nil {
			return nil, ErrClientRequired
		}
		if cfg.Storage == nil || cfg.Storage.Mongo == nil {
			return nil, ErrConfigRequired
		}
		return mongostorage.New(db,
			mongostorage.WithCollectionName(cfg.Storage.Mongo.Collection),
			mongostorage.WithTTL(cfg.TTL),
		)
	}
}
