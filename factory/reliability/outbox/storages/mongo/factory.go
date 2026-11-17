// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
	outboxmongo "github.com/altessa-s/go-atlas/reliability/outbox/storages/mongo"
)

var (
	// ErrDatabaseRequired is returned when the MongoDB outbox storage is built
	// without a database.
	ErrDatabaseRequired = errors.New("outbox: mongo database is required")
	// ErrCollectionRequired is returned when the MongoDB outbox storage is
	// built without a collection.
	ErrCollectionRequired = errors.New("outbox: mongo collection is required")
)

// Builder returns the storage function of the MongoDB outbox, ready for
// outboxfactory.OutboxBuilder.UseStorage(brokerconfig.OutboxStorageTypeMongo, ...).
// The storage keeps events in the collection of db named by
// outbox.storage.mongo.collection (the store's default when unset), creates
// its indexes on the build context and logs its watch failures on the
// builder's logger; the caller owns db.
func Builder(db *mongo.Database) outboxfactory.StorageFunc {
	return func(ctx context.Context, cfg *brokerconfig.Outbox, logger *slog.Logger) (outbox.Storage, error) {
		if db == nil {
			return nil, ErrDatabaseRequired
		}
		var collection string
		if cfg != nil && cfg.Storage != nil && cfg.Storage.Mongo != nil {
			collection = cfg.Storage.Mongo.Collection
		}
		//nolint:contextcheck // index creation runs on ctx via WithContext
		store, err := outboxmongo.New(db, outboxmongo.WithCollectionName(collection), outboxmongo.WithContext(ctx), outboxmongo.WithLogger(logger))
		if err != nil {
			return nil, err
		}
		return store, nil
	}
}

// CollectionBuilder returns the storage function of the MongoDB outbox over an
// existing collection, ready for outboxfactory.OutboxBuilder.UseStorage. The
// collection name in outbox.storage.mongo is ignored. The storage creates its
// indexes on the build context and logs its watch failures on the builder's
// logger; the caller owns col.
func CollectionBuilder(col *mongo.Collection) outboxfactory.StorageFunc {
	return func(ctx context.Context, _ *brokerconfig.Outbox, logger *slog.Logger) (outbox.Storage, error) {
		if col == nil {
			return nil, ErrCollectionRequired
		}
		//nolint:contextcheck // index creation runs on ctx via WithContext
		store, err := outboxmongo.NewWithCollectionOptions(col, outboxmongo.WithContext(ctx), outboxmongo.WithLogger(logger))
		if err != nil {
			return nil, err
		}
		return store, nil
	}
}
