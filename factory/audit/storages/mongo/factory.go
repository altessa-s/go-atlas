// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/audit"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongostorage "github.com/altessa-s/go-atlas/audit/storages/mongo"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	auditfactory "github.com/altessa-s/go-atlas/factory/audit"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// ErrDatabaseRequired is returned when the MongoDB audit storage is built
// without a database.
var ErrDatabaseRequired = corefactory.Errorf(corefactory.ErrDependencyRequired, "audit: mongo database is required for storage type mongo")

// Builder returns the storage builder of the mongo type, ready for
// auditfactory.AuditorBuilder.UseStorage. The storage writes to db; the
// caller owns it.
//
// The nested mongo section is optional: its absence means "all defaults", not
// an error, because the storage package supplies its own.
func Builder(db *mongo.Database) auditfactory.StorageBuilder {
	return func(ctx context.Context, cfg *auditconfig.Config, _ *slog.Logger) (audit.Storage, error) {
		if db == nil {
			return nil, ErrDatabaseRequired
		}

		var opts []mongostorage.Option
		if m := cfg.Storage.Mongo; m != nil {
			opts = coreslices.AppendIf(opts, m.CollectionName != "",
				mongostorage.WithCollectionName(m.CollectionName))
			opts = coreslices.AppendIf(opts, m.IndexTimeout > 0,
				mongostorage.WithIndexTimeout(m.IndexTimeout))
			opts = coreslices.AppendIf(opts, m.TTL > 0,
				mongostorage.WithTTL(m.TTL))
		}

		storage, err := mongostorage.New(ctx, db, opts...)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create audit mongo storage")
		}

		return storage, nil
	}
}
