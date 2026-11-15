// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
	horai "github.com/altessa-s/go-horai"
	redisstorage "github.com/altessa-s/go-horai/storage/redis"
)

var (
	// ErrClientRequired is returned when the Redis task storage is built
	// without a client.
	ErrClientRequired = errors.New("scheduler: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = errors.New("scheduler: redis configuration is required for storage type redis")
)

// ensureTimeout bounds the index creation the builder runs when the config
// asks for it, so a lock wait cannot stall startup indefinitely.
const ensureTimeout = 30 * time.Second

// Builder returns the task storage function of the redis type, ready for
// schedulerfactory.SchedulerBuilder.UseStorage. The storage keeps tasks and
// history through client; the caller owns it.
//
// With ensureIndexes set it creates the RediSearch indexes through the
// storage's EnsureIndexes; otherwise it performs no I/O and the indexes are
// expected to exist.
func Builder(client redis.UniversalClient) schedulerfactory.StorageFunc {
	return func(ctx context.Context, cfg *schedulerconfig.Config) (horai.Store, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		r := cfg.Storage.Redis
		if r == nil {
			return nil, ErrConfigRequired
		}

		storage, err := redisstorage.New(client,
			redisstorage.WithKeyPrefix(r.KeyPrefix),
			redisstorage.WithHistoryTTL(r.HistoryTTL),
			redisstorage.WithMaxHistoryPerTask(r.MaxHistoryPerTask),
		)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create scheduler storage")
		}
		if !r.EnsureIndexes {
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
