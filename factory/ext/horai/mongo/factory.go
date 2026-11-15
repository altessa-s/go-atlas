// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
	horai "github.com/altessa-s/go-horai"
	mongostorage "github.com/altessa-s/go-horai/storage/mongo"
)

var (
	// ErrDatabaseRequired is returned when the MongoDB task storage is built
	// without a database.
	ErrDatabaseRequired = errors.New("scheduler: mongo database is required for storage type mongo")
	// ErrConfigRequired is returned when the storage configuration has no
	// mongo block.
	ErrConfigRequired = errors.New("scheduler: mongo configuration is required for storage type mongo")
)

// ensureTimeout bounds the index creation the builder runs when the config
// asks for it, so a lock wait cannot stall startup indefinitely.
const ensureTimeout = 30 * time.Second

// Builder returns the task storage function of the mongo type, ready for
// schedulerfactory.SchedulerBuilder.UseStorage. The storage keeps tasks and
// history in db; the caller owns it.
//
// With ensureIndexes set it creates the indexes through the storage's
// EnsureIndexes; otherwise it performs no I/O and the indexes are expected to
// exist.
func Builder(db *mongo.Database) schedulerfactory.StorageFunc {
	return func(ctx context.Context, cfg *schedulerconfig.Config) (horai.Store, error) {
		if db == nil {
			return nil, ErrDatabaseRequired
		}
		m := cfg.Storage.Mongo
		if m == nil {
			return nil, ErrConfigRequired
		}

		storage, err := mongostorage.New(db,
			mongostorage.WithTasksCollection(m.TasksCollection),
			mongostorage.WithHistoryCollection(m.HistoryCollection),
		)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create scheduler storage")
		}
		if !m.EnsureIndexes {
			return storage, nil
		}

		ensureCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureTimeout)
		defer cancel()
		if err := storage.EnsureIndexes(ensureCtx); err != nil {
			return nil, coreerrs.WrapOperation(err, "ensure scheduler indexes")
		}
		return storage, nil
	}
}
