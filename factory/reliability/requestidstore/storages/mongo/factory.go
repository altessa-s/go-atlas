// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	config "github.com/altessa-s/go-atlas/config/requestidstore"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	storefactory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore"
	backend "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/mongo"
)

// Builder binds the mongo client for use with StoreBuilder.UseStorage.
func Builder(db *mongo.Database) storefactory.StorageFunc {
	return func(ctx context.Context, cfg *config.Config) (requestid.Store, error) {
		if nilcheck.IsNil(db) {
			return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "mongo client required")
		}
		if cfg == nil || cfg.Mongo == nil {
			return nil, corefactory.ErrConfigRequired
		}
		return backend.New(ctx, db, backend.WithCollectionName(cfg.Mongo.Collection))
	}
}
