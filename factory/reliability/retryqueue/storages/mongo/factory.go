// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
	mongostore "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/mongo"
)

var (
	// ErrClientRequired is returned when the MongoDB retry queue store is
	// built without a database. It wraps [corefactory.ErrDependencyRequired].
	ErrClientRequired = fmt.Errorf("retryqueue: MongoDB database is required for storage type mongo: %w", corefactory.ErrDependencyRequired)
	// ErrConfigRequired is returned when the storage configuration has no
	// mongo block. It wraps [corefactory.ErrConfigRequired].
	ErrConfigRequired = fmt.Errorf("retryqueue: mongo configuration is required for storage type mongo: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the mongo type, ready for
// retryqueuefactory.Builder.UseStorage. The store keeps items in the
// configured collection of db, with the configured flag fields and resolved
// retention; with ensureIndexes set, ctx bounds the index creation.
func Builder[P any](db *mongo.Database) retryqueuefactory.StorageFunc[P] {
	return func(ctx context.Context, cfg *retryqueueconfig.StorageConfig) (retryqueue.Storage[P], error) {
		if db == nil {
			return nil, ErrClientRequired
		}
		if cfg == nil || cfg.Mongo == nil {
			return nil, ErrConfigRequired
		}
		m := cfg.Mongo
		store, err := mongostore.New[P](db.Collection(m.Collection),
			mongostore.WithFlagField(m.FlagField),
			mongostore.WithFlagReasonField(m.FlagReasonField),
			mongostore.WithResolvedRetention(m.ResolvedRetention),
		)
		if err != nil {
			return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "create retry queue mongo store: %w", err)
		}
		if m.EnsureIndexes {
			if err := store.EnsureIndexes(ctx); err != nil {
				return nil, fmt.Errorf("ensure retry queue indexes: %w", err)
			}
		}
		return store, nil
	}
}
